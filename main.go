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

type Organization int

const (
	SingleFile Organization = iota
	ByChapters
	ByPages
)

type Page struct {
	Title    string
	Content  string
	URL      string
	Filename string
	Level    int
}

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
	singlePage   bool
}

func NewScraper(baseURL, outputPath string, minDelay, maxDelay float64, org Organization, singlePage bool) *Scraper {
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
		singlePage:   singlePage,
	}
}

func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.String()
}

func extractDomainPrefix(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

func sanitizeFilename(name string) string {
	invalid := []string{"/", "\\", "?", "%", "*", ":", "|", "\"", "<", ">", ".", " "}
	result := strings.ToLower(name)
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	if len(result) == 0 {
		result = "unnamed"
	}
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}

func (s *Scraper) humanizedDelay(noDelay bool) {
	if noDelay {
		return
	}
	delay := s.minDelay + rand.Float64()*(s.maxDelay-s.minDelay)
	delayDuration := time.Duration(delay * float64(time.Second))
	log.Printf("Pausing for %.3f seconds", delay)
	time.Sleep(delayDuration)
}

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

func (s *Scraper) shouldProcessURL(urlStr string) bool {
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		return false
	}

	checkURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	if checkURL.Host != baseURL.Host {
		return false
	}

	if s.visitedURLs[urlStr] {
		return false
	}

	relPath := strings.TrimPrefix(checkURL.Path, baseURL.Path)

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

	var title string
	if titleElem := mainContent.Find("h1").First(); titleElem.Length() > 0 {
		title = strings.TrimSpace(titleElem.Text())
		content.WriteString(fmt.Sprintf("# %s\n\n", title))
	}

	urlPath := strings.Trim(strings.TrimPrefix(url, s.baseURL), "/")
	level := len(strings.Split(urlPath, "/"))

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

func (s *Scraper) writeContentToFile(filepath string, content string) error {
	dir := path.Dir(filepath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(filepath, []byte(content), 0644)
}

func (s *Scraper) createIndex() error {
	indexPath := filepath.Join(s.outputDir, "index.md")
	var content strings.Builder

	title := strings.TrimPrefix(s.baseURL, "https://")
	title = strings.TrimPrefix(title, "http://")
	content.WriteString(fmt.Sprintf("# Documentation: %s\n\n", title))
	content.WriteString("## Table of Contents\n\n")

	for _, page := range s.pages {
		indent := strings.Repeat("  ", page.Level-1)
		content.WriteString(fmt.Sprintf("%s- [%s](%s) - [source](%s)\n",
			indent, page.Title, page.Filename, page.URL))
	}

	return s.writeContentToFile(indexPath, content.String())
}

func (s *Scraper) Scrape(noDelay bool) error {
	if s.singlePage {
		page, err := s.scrapePage(s.baseURL)
		if err != nil {
			return fmt.Errorf("error scraping page %s: %v", s.baseURL, err)
		}

		if s.organization == SingleFile {
			return s.writeContentToFile(s.outputPath, page.Content)
		}

		outputPath := filepath.Join(s.outputDir, page.Filename)
		return s.writeContentToFile(outputPath, page.Content)
	}

	links := []string{s.baseURL}
	processed := make(map[string]bool)
	var mainContent strings.Builder

	for len(links) > 0 {
		currentURL := links[0]
		links = links[1:]

		if processed[currentURL] {
			continue
		}

		log.Printf("Scraping: %s", currentURL)

		page, err := s.scrapePage(currentURL)
		if err != nil {
			log.Printf("Error scraping %s: %v", currentURL, err)
			continue
		}

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

		newLinks, err := s.getAllDocLinks(currentURL)
		if err != nil {
			log.Printf("Error getting links from %s: %v", currentURL, err)
			continue
		}

		for _, link := range newLinks {
			if !processed[link] {
				links = append(links, link)
			}
		}

		s.humanizedDelay(noDelay)
	}

	if s.organization == SingleFile {
		title := strings.TrimPrefix(s.baseURL, "https://")
		title = strings.TrimPrefix(title, "http://")
		header := fmt.Sprintf("# Documentation: %s\n\n", title)

		return s.writeContentToFile(s.outputPath, header+mainContent.String())
	} else {
		return s.createIndex()
	}
}

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
  -p, --single-page Scrape only the provided URL without following links
  --org             Organization type: single, chapters, pages [default: single]
  -h, --help        Display this help message

Organization Types:
  single            Create a single file containing all documentation
  chapters          Split documentation into chapter files
  pages             Split documentation into individual page files

Examples:
  docscrap -u https://nextjs.org/docs -o nextjs_doc.md
  docscrap -u https://react.dev/reference/react -o react_docs/doc.md --org pages
  docscrap -u https://docs.python.org/3/ -o python_doc.md -p`

	fmt.Println(helpText)
}

func main() {
	var (
		url        string
		output     string
		minDelay   float64
		maxDelay   float64
		help       bool
		noDelay    bool
		organize   string
		singlePage bool
	)

	flag.StringVar(&url, "u", "", "Documentation URL to scrape")
	flag.StringVar(&output, "o", "", "Output file path")
	flag.Float64Var(&minDelay, "min", 0.5, "Minimum delay between requests in seconds")
	flag.Float64Var(&maxDelay, "max", 5.0, "Maximum delay between requests in seconds")
	flag.BoolVar(&help, "h", false, "Display help message")
	flag.BoolVar(&noDelay, "n", false, "Disable delay between requests")
	flag.StringVar(&organize, "org", "single", "Organization type: single, chapters, pages")
	flag.BoolVar(&singlePage, "p", false, "Scrape only the provided URL without following links (shorthand)")

	flag.StringVar(&url, "url", "", "Documentation URL to scrape")
	flag.StringVar(&output, "output", "", "Output file path")
	flag.BoolVar(&help, "help", false, "Display help message")
	flag.BoolVar(&noDelay, "nodelay", false, "Disable delay between requests")
	flag.StringVar(&organize, "organization", "single", "Organization type: single, chapters, pages")
	flag.BoolVar(&singlePage, "single-page", false, "Scrape only the provided URL without following links")

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

	var org Organization
	switch strings.ToLower(organize) {
	case "chapters":
		org = ByChapters
	case "pages":
		org = ByPages
	default:
		org = SingleFile
	}

	rand.Seed(time.Now().UnixNano())

	scraper := NewScraper(url, output, minDelay, maxDelay, org, singlePage)
	if err := scraper.Scrape(noDelay); err != nil {
		log.Fatal(err)
	}
}
