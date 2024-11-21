```markdown
# 📚 DocScraper

> A modern CLI tool to convert online technical documentation into clean, organized Markdown files.

![Go Version](https://img.shields.io/badge/Go-1.16+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-green)
[![Go Report Card](https://goreportcard.com/badge/github.com/SellMindsAI/docscraper)](https://goreportcard.com/report/github.com/yourusername/docscraper)

## ✨ Features

- 🌐 Works with any documentation website
- 📑 Multiple organization options (single file, chapters, pages)
- 🚀 Smart delay system to prevent rate limiting
- 🎨 Clean Markdown output with preserved formatting
- 📁 Automatic content structuring
- 🧹 Removes navigation, ads, and other non-content elements
- 📝 Generates table of contents

## 🚀 Quick Start

```bash
# Install
go install github.com/yourusername/docscraper@latest

# Basic usage
docscrap -u https://nextjs.org/docs -o nextjs_doc.md
```

## 💻 Installation from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/docscraper
cd docscraper

# Install dependencies
go mod init docscraper
go get github.com/PuerkitoBio/goquery

# Build
go build -o docscrap
```

### Adding to PATH

#### Linux/macOS
```bash
# Option 1: Move to /usr/local/bin (requires sudo)
sudo mv docscrap /usr/local/bin/

# Option 2: Move to ~/bin (user specific)
mkdir -p ~/bin
mv docscrap ~/bin/
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.bashrc  # for bash
# OR
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc   # for zsh

# Reload shell configuration
source ~/.bashrc  # for bash
# OR
source ~/.zshrc   # for zsh
```

#### Windows
```powershell
# Option 1: Move to a directory that's already in PATH
move docscrap.exe C:\Windows\System32\

# Option 2: Add current directory to PATH
$env:Path += ";$pwd"
# To make it permanent, add the full path to System Environment Variables
```

## 📖 Usage

```bash
docscrap -u <url> -o <output_file> [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-u, --url` | Documentation URL to scrape | required |
| `-o, --output` | Output file path | required |
| `--org` | Organization type (single/chapters/pages) | single |
| `-min` | Minimum delay between requests (seconds) | 0.5 |
| `-max` | Maximum delay between requests (seconds) | 5.0 |
| `-n, --nodelay` | Disable request delays | false |

### Examples

```bash
# Single file output
docscrap -u https://nextjs.org/docs -o nextjs_doc.md

# Split into pages with custom delays
docscrap -u https://react.dev/reference/react \
         -o react_docs/doc.md \
         --org pages \
         -min 1.0 \
         -max 3.0

# Quick scraping with no delays
docscrap -u https://docs.python.org/3/ \
         -o python_doc.md \
         --nodelay
```

## 📁 Output Structure

### Single File
```
nextjs_doc.md
```

### Chapters Mode
```
docs/
├── index.md          # Table of contents
├── chapter1.md
├── chapter2.md
└── chapter3.md
```

### Pages Mode
```
docs/
├── index.md          # Table of contents
├── introduction/
│   ├── getting-started.md
│   └── installation.md
└── api/
    ├── overview.md
    └── reference.md
```

## 🛠️ Development

Requirements:
- Go 1.16+
- [goquery](https://github.com/PuerkitoBio/goquery)

Key files:
```
docscraper/
├── main.go           # Main application code
├── go.mod           # Go module file
├── go.sum           # Go module checksum
└── README.md        # Documentation
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🙏 Acknowledgments

- [goquery](https://github.com/PuerkitoBio/goquery) for HTML parsing
- The Go team for the amazing standard library

## ⚠️ Disclaimer

Please ensure you have permission to scrape content and respect websites' `robots.txt` files and rate limits.

---

Made with ❤️ by SellMindsAI
```