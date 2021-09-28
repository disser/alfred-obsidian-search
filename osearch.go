package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type AlfredResults struct {
	Items []AlfredResult `json:"items"`
}

type AlfredResult struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Arg      string `json:"arg"`
}

type RipGrepResult struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
	} `json:"data"`
}

func expandHome(filename string) string {
	if strings.HasPrefix(filename, "~/") {
		dir, _ := os.UserHomeDir()
		return filepath.Join(dir, filename[2:])
	}
	return filename
}

func findMatchingFiles(searchTerm string, directory string, vault string) AlfredResults {
	// TODO: set the environment, don't actually change directories
	err := os.Chdir(directory)
	if err != nil {
		log.Fatalf("no such directory %s", directory)
	}

	// TODO: don't hardcode the path to fd
	// TODO: sort the results in reverse chronological order
	out, err := exec.Command("/usr/local/bin/fd", "-0", "--type=f", "--glob", searchTerm).Output()
	if err != nil {
		log.Fatal(err)
	}

	output := strings.Split(string(out), "\000")
	results := make([]string, len(output))

	for index, filename := range output {
		results[index] = filename
	}

	alfredResults := make([]AlfredResult, len(results))

	for index, match := range results {
		if len(match) > 0 {
			alfredResults[index] = AlfredResult{
				Type:  "default",
				Title: withoutMd(filepath.Base(match)),
				Arg:   asObsidianUrl(match, vault),
			}
		}
	}

	return AlfredResults{Items: alfredResults}
}

func withoutMd(filename string) string {
	if strings.HasSuffix(filename, ".md") {
		return filename[0 : len(filename)-3]
	}
	return filename
}

func asObsidianUrl(path string, vault string) string {
	return fmt.Sprintf("obsidian://open?vault=%s&file=%s", vault, url.PathEscape(path))
}

// truncate something from the front
func fruncate(s string, p string, n int, m int) string {
	index := strings.Index(s, p)
	if index > n {
		max := index - n
		min := max - m
		breakIndex := strings.LastIndex(s[0:max], " ")
		if breakIndex > 0 && breakIndex >= min {
			return s[breakIndex+1:]
		}
		return s[max:]
	}
	return s
}

func grepMatchingFiles(searchTerm string, directory string, vault string) AlfredResults {
	err := os.Chdir(directory)
	if err != nil {
		log.Fatalf("no such directory %s", directory)
	}

	// TODO: don't hardcode the path to rg
	// TODO: sort in reverse chronological order
	out, err := exec.Command("/usr/local/bin/rg", "--json", "--sortr", "modified", searchTerm).Output()
	lines := strings.Split(string(out), "\n")

	var results []AlfredResult
	var rgr RipGrepResult
	alreadyFound := make(map[string]bool)
	for _, line := range lines {
		if !strings.HasPrefix(line, "{") {
			continue
		}
		//fmt.Println(line)
		err := json.Unmarshal([]byte(line), &rgr)
		if err != nil {
			log.Fatalf("could not parse %s", line)
		}

		if rgr.Type == "match" {
			filename := rgr.Data.Path.Text
			_, ok := alreadyFound[filename]
			if ok {
				continue
			}
			result := AlfredResult{
				Type:     "default",
				Title:    withoutMd(filepath.Base(filename)),
				Subtitle: fruncate(rgr.Data.Lines.Text, searchTerm, 10, 5),
				Arg:      asObsidianUrl(filename, vault),
			}
			results = append(results, result)
			alreadyFound[filename] = true
		}
	}

	return AlfredResults{
		Items: results,
	}
}

func main() {
	var grepMode bool
	var vaultName string
	var vaultPath string

	flag.BoolVar(&grepMode, "grep", false, "search file contents")
	flag.StringVar(&vaultName, "vault", "", "name of vault to search")
	flag.StringVar(&vaultPath, "path", "", "path to vault directory")
	flag.Parse()

	// TODO: Figure out if there is a way to find the current vault name and use that as default
	if len(vaultName) == 0 {
		log.Fatal("vault name must be specified with --vault")
	}

	// TODO: Figure out if there is a way to get the path from the vault name
	if len(vaultPath) == 0 {
		log.Fatal("vault path must be specified with --path")
	}

	var searchTerm string
	if len(flag.Args()) >= 1 {
		searchTerm = strings.Join(flag.Args(), " ")
	} else {
		log.Fatalf("Usage: %s [--grep] --vault vaultname --path vaultpath searchterm", os.Args[0])
	}

	var results AlfredResults
	if grepMode {
		results = grepMatchingFiles(searchTerm, expandHome(vaultPath), vaultName)
	} else {
		results = findMatchingFiles(searchTerm, expandHome(vaultPath), vaultName)
	}

	jsonResults, _ := json.MarshalIndent(results, "", "  ")
	// unescape the stupid ampersand
	jsonResults = []byte(strings.Replace(string(jsonResults), "\\u0026", "&", -1))
	fmt.Println(string(jsonResults))
}
