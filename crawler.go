package main

import (
	"bytes"
	"flag"
	"fmt"
	gq "github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Fetcher struct {
	InitUrl              string
	MaxDepth             int
	Destination          string
	LinksToProcess       map[string]bool
}

func NewFetcher(url string, max_depth int, dest string) (*Fetcher, error) {
    f := &Fetcher{
		InitUrl: url,
		MaxDepth: max_depth,
    	Destination: dest,
    	LinksToProcess: make(map[string]bool),
    }
    f.LinksToProcess[url] = false
    return f, nil
}

func (f *Fetcher) Crawl (url string, depth_level int) error {
	child_level := depth_level + 1
	if _, found := f.LinksToProcess[url]; found {
		// Set link to processed
		f.LinksToProcess[url] = true
	}
	url = AddHttpPrefixToUrlIfNeeded(url)
	resp, err := GetResponseFromUrl(url)
	if err != nil {
		return err
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg := fmt.Sprintf("Bad status code %d getting web page from url %s", resp.StatusCode, url)
			return errors.New(msg)
		}
		output, err := f.CreateFileFromLink(url)
		if err != nil {
			return errors.New(fmt.Sprintf("Error creating output file: %s\n", err.Error()))
		} else {
			defer output.Close()
			err = f.CopyToFileAndProcessLinks(output, resp)
		}
	}
	if f.MaxDepth <= 0 || depth_level < f.MaxDepth {
		for link, state := range f.LinksToProcess {
			if link != url && state == false {
				f.Crawl(link, child_level)
			}
		}
	}
	return nil
}

func (f *Fetcher) ProcessGoQueryElement(index int, element *gq.Selection) {
	href, exists := element.Attr("href")
	if exists {
		trimmedInitUrl := TrimSchemeIfNeeded(f.InitUrl)
		trimmedHref := TrimSchemeIfNeeded(href)
		http_href := strings.Replace(href, "https://", "http://", 1)
		httpInitUrl := strings.Replace(f.InitUrl, "https://", "http://", 1)
		if !(strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://"))  {
			// Handle relative links
			href = strings.TrimLeft(href, "/")
			if !strings.Contains(href, trimmedInitUrl){
				// Href is not a subdomain or subfolder of trimmed initial URL
				if len(href) > 0 && href[:1] != "/" {
					href = "/" + href
				}
				href = f.InitUrl + href
			}
			trimmedHref = TrimSchemeIfNeeded(href)
		}
		if (IsSubfolder(httpInitUrl, http_href) || IsSubfolder(trimmedInitUrl, trimmedHref)) {
			href = AddHttpPrefixToUrlIfNeeded(href)
			href = strings.TrimRight(href, "/")
			if _, found := f.LinksToProcess[href]; !found {
				f.LinksToProcess[href] = false
			}
		}
	}
}

func IsSubfolder(main_url string, link string) bool {
	return len(link) >= len(main_url) && link[:len(main_url)] == main_url
}

func AddHttpPrefixToUrlIfNeeded(url string) string {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		url = "http://" + url
	}
	return url
}

func TrimSchemeIfNeeded(url string) string {
	if strings.HasPrefix(url, "https://") {
		url = url[len("https://"):]
	} else if strings.HasPrefix(url, "http://") {
		url = url[len("http://"):]
	}
	return url
}

func StripScheme(url string) string {
	if strings.HasPrefix(url, "https://") {
		return url[len("https://"):len(url)]
	} else if strings.HasPrefix(url, "http://"){
		return url[len("http://"):len(url)]
	}
	return url
}

func (f *Fetcher) CreateFileFromLink(url string) (*os.File, error) {
	var file_path string
	if url == f.InitUrl {
		// If this is the initial URL, make folder in downloads dir and create index.html
		file_path = filepath.Join(f.Destination, StripScheme(url))
		err := os.Mkdir(file_path, os.ModeDir)
		if os.IsExist(err) {
			err = os.Chmod(file_path, 0755)
			if err != nil {
				fmt.Printf("Could not change permissions on %s directory: %s\n", file_path, err.Error())
			}
		} else if err != nil {
			fmt.Printf("Could not create %s site directory\n", file_path)
		}
		file_path = filepath.Join(file_path, "index.html")
	} else {
		path_fragments := strings.Split(StripScheme(url), "/")
		file_name := path_fragments[len(path_fragments)-1]
		folders := StripScheme(url)[:len(StripScheme(url))-len(file_name)]
		file_path = filepath.Join(f.Destination, folders)
		err := os.MkdirAll(file_path, os.ModeDir)
		if err != nil {
			fmt.Printf("Could not create %s site directory: %s\n", file_path, err.Error())
		}
		err = os.Chmod(file_path, 0755)
		if err != nil {
			fmt.Printf("Could not change permissions on %s directory: %s\n", file_path, err.Error())
		}
		file_path = filepath.Join(file_path, file_name)
		file_path = strings.ReplaceAll(file_path, "&", "_")
		file_path = strings.ReplaceAll(file_path, "?", "_")
	}
	output, err := os.Create(file_path)
	if err != nil {
		fmt.Printf("Could not create %s file: %s\n", file_path, err.Error())
	}
	perms := os.FileMode(0755)
	err = os.Chmod(file_path, perms)
	if err != nil {
		fmt.Printf("Could not change permissions on %s directory: %s\n", file_path, err.Error())
	}
	return output, nil
}

func GetResponseFromUrl(url string) (http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return http.Response{}, errors.New(fmt.Sprintf("Error making web request: %s", err.Error()))
	}
	return *resp, nil
}

func (f *Fetcher) CopyToFileAndProcessLinks(output *os.File, resp http.Response) error {
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	_, err := output.Write(bodyBytes)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not write page contents to file: %s\n", err.Error()))
	}
	//reset the response body to the original unread state
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	document, err := gq.NewDocumentFromReader(resp.Body)
	if err != nil {
		return errors.New(fmt.Sprintf("Error loading HTTP response body into document: %s\n", err.Error()))
	}
	document.Find("a").Each(f.ProcessGoQueryElement)
	return nil
}

func main () {
	initTime := time.Now()
	url := flag.String("url", "", "a URL to crawl")
	dest := flag.String("dest", "downloads", "Destination directory")
	max_depth := flag.Int("max_depth", -1, "Max link depth to crawl")
	flag.Parse()
	if *url == "" {
		fmt.Printf("Please enter a URL to crawl\n")
		return
	} else {
		err := os.Mkdir(*dest, os.ModeDir)
		if os.IsExist(err) {
			err = os.Chmod(*dest, 0755)
			if err != nil {
				fmt.Printf("Could not change permissions on %s directory: %s\n", *dest, err.Error())
			}
		} else if err != nil {
			fmt.Printf("Could not create destination directory\n")
			return
		}
		f, _ := NewFetcher(*url, *max_depth, *dest)
		f.Crawl(f.InitUrl, 1)
	}
	fmt.Printf("Script start time: %s\n", initTime)
	fmt.Printf("Script end time: %s\n", time.Now())
}
