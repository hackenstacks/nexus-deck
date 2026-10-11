package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bolt "go.etcd.io/bbolt"
)

const nexusDBPath = "nexus.db"
const dbFilePerm = 0600

// OpenNexusDB opens (or creates) the single bbolt database file.
func OpenNexusDB() (*bolt.DB, error) {
	db, err := bolt.Open(nexusDBPath, dbFilePerm, nil)
	if err != nil {
		return nil, err
	}

	// Create all required buckets
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := []string{"_meta", "characters", "sessions", "memories", "images", "library", "vault", "lorebooks", "knowledge", "configs", "library_metadata"}
		for _, b := range buckets {
			_, err := tx.CreateBucketIfNotExists([]byte(b))
			if err != nil {
				return fmt.Errorf("create bucket %s: %s", b, err)
			}
		}
		return nil
	})

	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// DBSaveAuth writes [salt][encUserData] into _meta/auth bucket.
func DBSaveAuth(db *bolt.DB, salt []byte, encUserData []byte) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("_meta"))
		// Store as [salt][encData]
		combined := make([]byte, len(salt)+len(encUserData))
		copy(combined, salt)
		copy(combined[len(salt):], encUserData)
		return b.Put([]byte("auth"), combined)
	})
}

// DBLoadAuth reads _meta/auth → returns salt, encUserData.
func DBLoadAuth(db *bolt.DB) ([]byte, []byte, error) {
	var salt, encData []byte
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("_meta"))
		if b == nil {
			return fmt.Errorf("meta bucket not found")
		}
		val := b.Get([]byte("auth"))
		if val == nil {
			return fmt.Errorf("auth record not found")
		}
		if len(val) < 16 {
			return fmt.Errorf("invalid auth record")
		}
		salt = make([]byte, 16)
		copy(salt, val[:16])
		encData = make([]byte, len(val)-16)
		copy(encData, val[16:])
		return nil
	})
	return salt, encData, err
}

// DBPut encrypts value with encKey and stores it in bucket under key.
func DBPut(db *bolt.DB, encKey []byte, bucket string, key string, value []byte) error {
	encrypted, err := Encrypt(value, encKey)
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		return b.Put([]byte(key), encrypted)
	})
}

// DBGet retrieves and decrypts a value from bucket by key.
func DBGet(db *bolt.DB, encKey []byte, bucket string, key string) ([]byte, error) {
	var encrypted []byte
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		val := b.Get([]byte(key))
		if val == nil {
			return fmt.Errorf("key %s not found in bucket %s", key, bucket)
		}
		encrypted = make([]byte, len(val))
		copy(encrypted, val)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return Decrypt(encrypted, encKey)
}

// DBDelete removes a key from bucket.
func DBDelete(db *bolt.DB, bucket string, key string) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		return b.Delete([]byte(key))
	})
}

// DBList returns all keys in a bucket.
func DBList(db *bolt.DB, bucket string) ([]string, error) {
	var keys []string
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		return b.ForEach(func(k, v []byte) error {
			keys = append(keys, string(k))
			return nil
		})
	})
	return keys, err
}

// DBExists returns true if a key exists in a bucket.
func DBExists(db *bolt.DB, bucket string, key string) bool {
	exists := false
	_ = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b != nil {
			if b.Get([]byte(key)) != nil {
				exists = true
			}
		}
		return nil
	})
	return exists
}

// MigrateUserToDBIfNeeded checks for old flat-file data and migrates to bbolt.
func MigrateUserToDBIfNeeded(user *User) error {
	userDataPath := GetUserDataPath(user.Username)
	if _, err := os.Stat(userDataPath); os.IsNotExist(err) {
		return nil // Nothing to migrate
	}

	NexusLog("INFO", "db", "Legacy data detected. Starting migration to bbolt...")

	// Map old data types to new bucket names
	typeMap := map[string]string{
		"characters":      "characters",
		"chat_sessions":   "sessions",
		"memories":        "memories",
		"images":          "images",
		"library_content": "library",
		"vault":           "vault",
		"vault_content":   "vault", // Note: combined into vault bucket
		"global/lorebooks": "lorebooks",
		"knowledge":       "knowledge",
	}

	for oldType, bucket := range typeMap {
		oldDirPath := filepath.Join(userDataPath, oldType)
		if _, err := os.Stat(oldDirPath); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(oldDirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(info.Name(), ".bin") {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			key := strings.TrimSuffix(info.Name(), ".bin")
			
			err = user.DB.Update(func(tx *bolt.Tx) error {
				b := tx.Bucket([]byte(bucket))
				return b.Put([]byte(key), data)
			})
			
			if err == nil {
				NexusLog("INFO", "db", fmt.Sprintf("Migrated %s/%s", bucket, key))
			}
			return err
		})

		if err != nil {
			return fmt.Errorf("migration failed for %s: %w", oldType, err)
		}
	}

	NexusLog("INFO", "db", "Migration complete. Nexus is now running on bbolt.")

	// Rename legacy data dir so migration never runs again
	migratedPath := userDataPath + ".migrated"
	if err := os.Rename(userDataPath, migratedPath); err != nil {
		NexusLog("WARN", "db", "Could not rename legacy data dir after migration: "+err.Error())
	} else {
		NexusLog("INFO", "db", "Legacy data archived to "+migratedPath)
	}
	return nil
}
