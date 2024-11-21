package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Organization defines how the documentation will be organized in files
type Organization int

const (
	SingleFile Organization = iota
	ByChapters
	ByPages
)

// Page represents a single documentation page
type Page struct {
	Title    string
	Content  string
	URL      string
	Filename string
	Level    int
}

// Scraper holds all the necessary configuration and state for scraping
type Scraper struct {
	baseURL      string
	outputPath   string
	outputDir    string
	visitedURLs  map[string]bool
	client       *http.Client
	domainPrefix string
	minDelay     float64
	maxDelay     float64
	organization Organization
	pages        []Page
}

// NewScraper creates and initializes a new Scraper instance
func NewScraper(baseURL, outputPath string, minDelay, maxDelay float64, org Organization) *Scraper {
	return &Scraper{
		baseURL:     sanitizeURL(baseURL),
		outputPath:  outputPath,
		outputDir:   strings.TrimSuffix(outputPath, filepath.Ext(outputPath)),
		visitedURLs: make(map[string]bool),
		client: &http.Client{
			Timeout: time.Second * 30,
		},
		domainPrefix: extractDomainPrefix(baseURL),
		minDelay:     minDelay,
		maxDelay:     maxDelay,
		organization: org,
		pages:        make([]Page, 0),
	}
}

// sanitizeURL ensures the URL is properly formatted
func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.String()
}

// extractDomainPrefix gets the base domain from a URL
func extractDomainPrefix(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

// sanitizeFilename creates a safe filename from a string
func sanitizeFilename(name string) string {
	// Replace invalid characters with underscores
	invalid := []string{"/", "\\", "?", "%", "*", ":", "|", "\"", "<", ">", ".", " "}
	result := strings.ToLower(name)
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	// Ensure the filename is not empty and has a reasonable length
	if len(result) == 0 {
		result = "unnamed"
	}
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}

// humanizedDelay introduces a random delay between requests
func (s *Scraper) humanizedDelay(noDelay bool) {
	if noDelay {
		return
	}
	delay := s.minDelay + rand.Float64()*(s.maxDelay-s.minDelay)
	delayDuration := time.Duration(delay * float64(time.Second))
	log.Printf("Pausing for %.3f seconds", delay)
	time.Sleep(delayDuration)
}

// resolveURL converts relative URLs to absolute URLs
func (s *Scraper) resolveURL(href string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	if strings.HasPrefix(href, "//") {
		u, err := url.Parse(s.baseURL)
		if err != nil {
			return ""
		}
		return u.Scheme + ":" + href
	}
	if strings.HasPrefix(href, "/") {
		return s.domainPrefix + href
	}
	return s.baseURL + "/" + href
}

// shouldProcessURL determines if a URL should be scraped
func (s *Scraper) shouldProcessURL(urlStr string) bool {
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		return false
	}

	checkURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Check if URLs have the same host
	if checkURL.Host != baseURL.Host {
		return false
	}

	// Check if already visited
	if s.visitedURLs[urlStr] {
		return false
	}

	// Get the path relative to the base URL
	relPath := strings.TrimPrefix(checkURL.Path, baseURL.Path)

	// Ignore common non-documentation paths
	ignorePaths := []string{
		"/assets/", "/static/", "/img/", "/images/",
		"/js/", "/css/", "/fonts/", "/examples/",
		"/blog/", "/community/", "/download/",
	}

	for _, ignore := range ignorePaths {
		if strings.Contains(relPath, ignore) {
			return false
		}
	}

	return true
}

// getAllDocLinks retrieves all documentation links from a page
func (s *Scraper) getAllDocLinks(currentURL string) ([]string, error) {
	req, err := http.NewRequest("GET", currentURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var links []string
	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		if href, exists := sel.Attr("href"); exists {
			fullURL := s.resolveURL(href)
			if fullURL != "" && s.shouldProcessURL(fullURL) {
				links = append(links, fullURL)
			}
		}
	})

	return links, nil
}

// scrapePage extracts content from a single page
func (s *Scraper) scrapePage(url string) (page Page, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return Page{}, err
	}

	// Remove non-content elements
	doc.Find(`
		header, footer, nav, 
		.header, .footer, .navigation, .nav, .navbar,
		.sidebar, .side-bar, .menu, .toc,
		.ad, .ads, .advertisement,
		.cookie-banner, .cookies,
		.search, .searchbox,
		[role="banner"], [role="navigation"],
		.social-links, .share-buttons
	`).Remove()

	var content strings.Builder
	content.WriteString(fmt.Sprintf("\n## Source: %s\n\n", url))

	// Find main content
	mainContent := doc.Find(`
		article, 
		main, 
		[role="main"],
		.main-content,
		.content,
		.article,
		.post,
		.documentation,
		.doc-content,
		#content,
		#main
	`).First()

	if mainContent.Length() == 0 {
		mainContent = doc.Find("body")
	}

	// Extract title and content
	var title string
	if titleElem := mainContent.Find("h1").First(); titleElem.Length() > 0 {
		title = strings.TrimSpace(titleElem.Text())
		content.WriteString(fmt.Sprintf("# %s\n\n", title))
	}

	// Calculate page level from URL structure
	urlPath := strings.Trim(strings.TrimPrefix(url, s.baseURL), "/")
	level := len(strings.Split(urlPath, "/"))

	// Process content elements
	mainContent.Find("h2, h3, h4, h5, h6, p, pre, ul, ol, code, blockquote").Each(func(i int, sel *goquery.Selection) {
		switch goquery.NodeName(sel) {
		case "h2", "h3", "h4", "h5", "h6":
			level := int(sel.Get(0).Data[1] - '0')
			text := strings.TrimSpace(sel.Text())
			if text != "" {
				content.WriteString(fmt.Sprintf("%s %s\n\n", strings.Repeat("#", level), text))
			}
		case "p":
			text := strings.TrimSpace(sel.Text())
			if text != "" {
				content.WriteString(text + "\n\n")
			}
		case "pre":
			if codeBlock := sel.Find("code"); codeBlock.Length() > 0 {
				lang := ""
				if className, exists := codeBlock.Attr("class"); exists {
					langClasses := []string{"language-", "lang-", "brush:"}
					for _, prefix := range langClasses {
						if strings.Contains(className, prefix) {
							parts := strings.Split(className, prefix)
							if len(parts) > 1 {
								lang = strings.Split(parts[1], " ")[0]
								break
							}
						}
					}
				}
				code := strings.TrimSpace(codeBlock.Text())
				if code != "" {
					if lang != "" {
						content.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", lang, code))
					} else {
						content.WriteString(fmt.Sprintf("```\n%s\n```\n\n", code))
					}
				}
			}
		case "ul", "ol":
			sel.Find("li").Each(func(_ int, li *goquery.Selection) {
				text := strings.TrimSpace(li.Text())
				if text != "" {
					content.WriteString("- " + text + "\n")
				}
			})
			content.WriteString("\n")
		case "blockquote":
			text := strings.TrimSpace(sel.Text())
			if text != "" {
				for _, line := range strings.Split(text, "\n") {
					content.WriteString("> " + strings.TrimSpace(line) + "\n")
				}
				content.WriteString("\n")
			}
		}
	})

	content.WriteString("---\n\n")

	// Create filename from title or URL
	filename := sanitizeFilename(title)
	if filename == "" {
		filename = sanitizeFilename(filepath.Base(url))
	}
	filename += ".md"

	return Page{
		Title:    title,
		Content:  content.String(),
		URL:      url,
		Filename: filename,
		Level:    level,
	}, nil
}

// writeContentToFile writes content to a file and creates necessary directories
func (s *Scraper) writeContentToFile(filepath string, content string) error {
	dir := path.Dir(filepath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(filepath, []byte(content), 0644)
}

// createIndex creates an index file with links to all pages
func (s *Scraper) createIndex() error {
	indexPath := filepath.Join(s.outputDir, "index.md")
	var content strings.Builder

	// Write header
	title := strings.TrimPrefix(s.baseURL, "https://")
	title = strings.TrimPrefix(title, "http://")
	content.WriteString(fmt.Sprintf("# Documentation: %s\n\n", title))
	content.WriteString("## Table of Contents\n\n")

	// Sort pages by level and write links
	for _, page := range s.pages {
		indent := strings.Repeat("  ", page.Level-1)
		content.WriteString(fmt.Sprintf("%s- [%s](%s) - [source](%s)\n",
			indent, page.Title, page.Filename, page.URL))
	}

	return s.writeContentToFile(indexPath, content.String())
}

// Scrape manages the complete scraping process
func (s *Scraper) Scrape(noDelay bool) error {
	links := []string{s.baseURL}
	processed := make(map[string]bool)
	var mainContent strings.Builder

	// Process links breadth-first
	for len(links) > 0 {
		currentURL := links[0]
		links = links[1:]

		if processed[currentURL] {
			continue
		}

		log.Printf("Scraping: %s", currentURL)

		// Scrape current page
		page, err := s.scrapePage(currentURL)
		if err != nil {
			log.Printf("Error scraping %s: %v", currentURL, err)
			continue
		}

		// Handle content based on organization type
		switch s.organization {
		case SingleFile:
			mainContent.WriteString(page.Content)
		case ByChapters, ByPages:
			outputPath := filepath.Join(s.outputDir, page.Filename)
			if err := s.writeContentToFile(outputPath, page.Content); err != nil {
				log.Printf("Error writing file %s: %v", outputPath, err)
			}
		}

		s.pages = append(s.pages, page)
		processed[currentURL] = true

		// Get new links
		newLinks, err := s.getAllDocLinks(currentURL)
		if err != nil {
			log.Printf("Error getting links from %s: %v", currentURL, err)
			continue
		}

		// Add new unprocessed links
		for _, link := range newLinks {
			if !processed[link] {
				links = append(links, link)
			}
		}

		s.humanizedDelay(noDelay)
	}

	// Write final output
	if s.organization == SingleFile {
		title := strings.TrimPrefix(s.baseURL, "https://")
		title = strings.TrimPrefix(title, "http://")
		header := fmt.Sprintf("# Documentation: %s\n\n", title)

		return s.writeContentToFile(s.outputPath, header+mainContent.String())
	} else {
		return s.createIndex()
	}
}

// printHelp displays usage information
func printHelp() {
	helpText := `Universal Documentation Scraper

Usage:
  docscrap -u <url> -o <output_file> [options]

Options:
  -u, --url         Documentation URL to scrape
  -o, --output      Output file path (Markdown format)
  -min              Minimum delay between requests in seconds [default: 0.5]
  -max              Maximum delay between requests in seconds [default: 5.0]
  -n, --nodelay     Disable delay between requests
  --org             Organization type: single, chapters, pages [default: single]
  -h, --help        Display this help message

Organization Types:
  single            Create a single file containing all documentation
  chapters          Split documentation into chapter files
  pages             Split documentation into individual page files

Examples:
  docscrap -u https://nextjs.org/docs -o nextjs_doc.md
  docscrap -u https://react.dev/reference/react -o react_docs/doc.md --org pages
  docscrap -u https://docs.python.org/3/ -o python_doc.md --org chapters

Note:
  When using 'chapters' or 'pages' organization, the output path will be used
  as the base directory for the documentation files.`

	fmt.Println(helpText)
}

func main() {
	var (
		url      string
		output   string
		minDelay float64
		maxDelay float64
		help     bool
		noDelay  bool
		organize string
	)

	// Configure command line flags
	flag.StringVar(&url, "u", "", "Documentation URL to scrape")
	flag.StringVar(&output, "o", "", "Output file path")
	flag.Float64Var(&minDelay, "min", 0.5, "Minimum delay between requests in seconds")
	flag.Float64Var(&maxDelay, "max", 5.0, "Maximum delay between requests in seconds")
	flag.BoolVar(&help, "h", false, "Display help message")
	flag.BoolVar(&noDelay, "n", false, "Disable delay between requests")
	flag.StringVar(&organize, "org", "single", "Organization type: single, chapters, pages")

	// Long versions of flags
	flag.StringVar(&url, "url", "", "Documentation URL to scrape")
	flag.StringVar(&output, "output", "", "Output file path")
	flag.BoolVar(&help, "help", false, "Display help message")
	flag.BoolVar(&noDelay, "nodelay", false, "Disable delay between requests")
	flag.StringVar(&organize, "organization", "single", "Organization type: single, chapters, pages")

	flag.Parse()

	if help {
		printHelp()
		os.Exit(0)
	}

	if url == "" || output == "" {
		fmt.Println("Error: URL and output file are required")
		printHelp()
		os.Exit(1)
	}

	if minDelay > maxDelay {
		log.Fatal("Minimum delay must be less than maximum delay")
	}

	// Determine organization type
	var org Organization
	switch strings.ToLower(organize) {
	case "chapters":
		org = ByChapters
	case "pages":
		org = ByPages
	default:
		org = SingleFile
	}

	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	// Create and run scraper
	scraper := NewScraper(url, output, minDelay, maxDelay, org)
	if err := scraper.Scrape(noDelay); err != nil {
		log.Fatal(err)
	}
}
