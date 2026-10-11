# 🎨 GEMINI TASK — Image Generation Settings UI

**Project:** `/home/user/claude/nexus-network-stack/deck/`
**Language:** Go · Bubble Tea / Lip Gloss TUI

---

## 🔴 RULES

1. **Backup every file before touching it:** `cp file.go file.go.bak-$(date +%Y%m%d)`
2. No mocks — all settings must wire through to actual image generation calls
3. Use emojis for all labels and section headers
4. Build must pass: `go build .` from the deck directory

---

## 🎯 TASK — Image Style Settings Panel

Add an **Image Settings** section inside the existing Settings tab (`TabSettings` → `api_settings.go`).

### Step 1 — Add fields to `APIConfig` in `api_config.go`

Add this struct and field to `APIConfig`:

```go
// ImageStyleConfig holds user preferences for image generation style
type ImageStyleConfig struct {
    StylePreset    string `json:"stylePreset"`    // e.g. "anime", "digital art"
    NegativePrompt string `json:"negativePrompt"` // e.g. "blurry, watermark, text"
    AspectRatio    string `json:"aspectRatio"`    // "portrait", "square", "landscape"
    Width          int    `json:"width"`          // pixel width
    Height         int    `json:"height"`         // pixel height
}
```

Add to `APIConfig` struct:
```go
ImageStyle ImageStyleConfig `json:"imageStyle"`
```

Add sensible defaults in `DefaultAPIConfig()`:
```go
ImageStyle: ImageStyleConfig{
    StylePreset:    "digital art",
    NegativePrompt: "blurry, low quality, watermark, text, ugly, deformed",
    AspectRatio:    "square",
    Width:          512,
    Height:         512,
},
```

---

### Step 2 — Wire settings into image prompt in `chat_commands.go` and `chat.go`

Wherever `GenerateImage(ctx, prompt, model)` is called, prepend the style preset and append the negative prompt using the config:

```go
// Before calling GenerateImage — build styled prompt
styledPrompt := buildStyledImagePrompt(prompt, m.apiConfig.ImageStyle)
```

Add this helper function (new file `image_style.go` or bottom of `chat_commands.go`):

```go
func buildStyledImagePrompt(base string, style ImageStyleConfig) string {
    prompt := base
    if style.StylePreset != "" {
        prompt = style.StylePreset + ", " + prompt
    }
    if style.NegativePrompt != "" {
        prompt += " | negative: " + style.NegativePrompt
    }
    return prompt
}
```

Also pass `Width` and `Height` to providers that support it (Pollinations accepts `?width=&height=` query params — update `PollinationsImageService.GenerateImage()` in `services.go` to read from config or accept size params).

---

### Step 3 — Add Image Settings UI in `api_settings.go`

Add a new settings section rendered after existing provider settings. Use numerical selection menus consistent with the existing settings UI style.

**Menu structure:**

```
🎨 IMAGE STYLE SETTINGS
────────────────────────────────────
🖼️  Style Preset      [1] Anime  [2] Digital Art  [3] Photorealistic
                       [4] Oil Painting  [5] Watercolor  [6] Cyberpunk
                       [7] Fantasy  [8] Sketch  [9] Custom (enter text)

📐 Aspect Ratio       [1] Portrait (512×768)
                       [2] Square (512×512)
                       [3] Landscape (768×512)
                       [4] HD Portrait (768×1024)
                       [5] HD Landscape (1024×768)

🚫 Negative Prompt    [current value — editable text input]

💾 [S] Save Settings
```

**Style presets with their Width/Height:**
| Selection | Preset | W | H |
|-----------|--------|---|---|
| 1 | anime | 512 | 768 |
| 2 | digital art | 512 | 512 |
| 3 | photorealistic | 512 | 512 |
| 4 | oil painting | 512 | 512 |
| 5 | watercolor | 512 | 512 |
| 6 | cyberpunk | 768 | 512 |
| 7 | fantasy | 512 | 768 |
| 8 | sketch | 512 | 512 |
| 9 | custom | keep current W/H |

**Aspect ratio selections set both Width AND Height.**

---

### Step 4 — Persist settings

`APIConfig` already saves/loads via JSON. Since `ImageStyleConfig` is added to `APIConfig`, it will persist automatically when `SaveAPIConfig()` is called. No extra work needed — just make sure [S] Save triggers the existing save function.

---

## ✅ Delivery

- [ ] Backups created for all modified files
- [ ] `go build .` passes clean
- [ ] Selecting a style preset updates Width + Height automatically
- [ ] Negative prompt is editable text input
- [ ] Style is prepended + negative prompt appended on every `/image` call and auto-gen
- [ ] Pollinations `?width=&height=` query params updated from config
- [ ] Report what was done at the bottom of this file under `## COMPLETED`

---

## COMPLETED

*(Gemini fills this in)*
