---
name: hugo-cli
description: Executes Hugo CLI commands for local development, building, and content management. Use for scaffolding Hugo projects, running dev server, building static sites, creating new content, and running Hugo commands in this project.
---

# Hugo CLI Skill

Commands for managing the tts-go Hugo site. Hugo is installed locally via Go, and the repo should use `make` as the canonical top-level interface.

## Development Server

```bash
make dev
```
Starts dev server on `http://localhost:1313`. Rebuilds on file changes.

## Building

**Development build:**
```bash
make build
```

**Production build (minified + GC):**
```bash
make build-prod
```

Output: `public/` directory

## Content Generation

**Create new page content:**
```bash
hugo new content/path/to/page/index.md
```

Examples:
```bash
hugo new content/about/index.md
hugo new content/services/index.md
hugo new content/service-areas/lees-summit/index.md
```

**Create new post (not typically used here, but available):**
```bash
hugo new posts/my-post.md
```

## Inspect Site

**Check Hugo config:**
```bash
hugo config
```

**List all content:**
```bash
hugo list all
```

**View site details:**
```bash
hugo list drafts    # Show draft content
hugo list expired   # Show expired content
hugo list future    # Show future-dated content
```

## Cleanup

**Remove generated files:**
```bash
make clean
```

Deletes `resources/_gen` and `public/` directories. Safe to run before rebuilding.

## Tooling Conventions

- Prefer Make targets for top-level workflows so humans and agents use the same entrypoints
- Do not add `package.json`/Bun unless real JS dependencies justify them later
- Prefer small Go tools under `cmd/<tool>/main.go` for non-trivial helper scripts
- Add `go.mod` only when the first Go helper tool is introduced

## Project Structure

The site follows this layout:
```
tts-go/
├── Makefile          # Canonical dev/build/clean commands
├── README.md         # Setup and workflow docs
├── hugo.toml         # Hugo configuration
├── content/          # 📝 Markdown pages
├── layouts/          # 🏗️ HTML templates
├── assets/           # 🎨 CSS/JS processed by Hugo Pipes
├── static/           # 📦 Static files (copied as-is)
├── data/             # 📊 YAML/JSON data files
└── cmd/              # Optional Go helper tools
```

## Common Workflows

### Create a new page with metadata
```bash
hugo new content/faq/index.md
# Edit content/faq/index.md with front matter and markdown
make dev
# Visit http://localhost:1313/faq/
```

### Build for production
```bash
make build-prod
# Output in public/ ready for deployment to Cloudflare Pages
```

### Debug a page
```bash
make dev
# Edit layouts/ and assets/ files
# Browser auto-refreshes (disable cache or use DevTools)
```

### Check for broken templates
```bash
hugo server --logLevel info
```

## Configuration

Hugo config: `hugo.toml`

Key settings:
- `baseURL` — site base URL (set per environment)
- `languageCode` — language/locale
- `title` — site title
- `outputs` — output formats (HTML, RSS, etc.)
- `menu` — site navigation menus
- `params` — custom site parameters

## Tips

- Hugo watches `content/`, `layouts/`, `static/`, `data/`, `assets/` by default
- Changes to `hugo.toml` require server restart
- Draft content (`draft: true` in front matter) only appears with `--buildDrafts`
- Use `hugo server --disableFastRender` for full rebuilds (slower but safer)
