package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"

	"golang.org/x/oauth2"

	"github.com/alecthomas/kingpin"
	"github.com/cheggaaa/pb"
	"github.com/google/go-github/github"
)

var (
	suggestCommand = kingpin.Command("suggest", "suggest repository")
	suggestFrom    = suggestCommand.Arg("from", "repository name").Required().String()
	ignoreCommand  = kingpin.Command("ignore", "ignore repository")
	ignoreTarget   = ignoreCommand.Arg("ignore_target", "repository name to ignore").Required().String()
)

type config struct {
	Token   string   `json:"token"`
	PerPage int      `json:"per_page"`
	Ignore  []string `json:"ignore"`
}

func loadConfig() *config {
	c := config{}
	path := c.Path()
	b, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("failed to read config `%s`: %s", path, err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		log.Fatalf("failed to parse config json `%s`: %s", path, err)
	}
	return &c
}

func (c *config) Path() string {
	return fmt.Sprintf("%s/.starlink.json", os.Getenv("HOME"))
}

func (c *config) Save() {
	b, _ := json.MarshalIndent(c, "", "  ")
	path := c.Path()
	if err := ioutil.WriteFile(path, b, 0644); err != nil {
		log.Fatalf("failed to save config json `%s`:%s", path, err)
	}
}

func getOwnerAndRepo(in string) (string, string) {
	tokens := strings.Split(in, "/")
	last := len(tokens) - 1
	return tokens[last-1], tokens[last]
}

func main() {
	switch kingpin.Parse() {
	case "suggest":
		suggestMain(*suggestFrom)
	case "ignore":
		ignoreMain(*ignoreTarget)
	}
}

func suggestMain(from string) {
	c := loadConfig()
	s := StarLink{
		token:   c.Token,
		perpage: c.PerPage,
	}
	s.setup()
	bar := pb.New(s.perpage)
	bar.Output = os.Stderr
	bar.Start()
	owner, repo := getOwnerAndRepo(from)
	list := s.suggest(owner, repo, func() { bar.Increment() })
	bar.Finish()
	ignores := map[string]struct{}{}
	for _, ign := range c.Ignore {
		ignores[ign] = struct{}{}
	}
	for _, name := range list {
		if _, ok := ignores[name]; !ok {
			fmt.Printf("http://github.com/%s\n", name)
		}
	}
}

func ignoreMain(target string) {
	c := loadConfig()
	owner, repo := getOwnerAndRepo(target)
	name := fmt.Sprintf("%s/%s", owner, repo)
	c.Ignore = append(c.Ignore, name)
	c.Save()
	fmt.Fprintf(os.Stderr, "added %s to ignore list\n", name)
}

type StarLink struct {
	token   string
	perpage int
	client  *github.Client
}

func (s *StarLink) setup() {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: s.token},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)

	s.client = github.NewClient(tc)
}

func (s *StarLink) starred(owner string) []github.Repository {
	repos, _, err := s.client.Activity.ListStarred(owner, &github.ActivityListStarredOptions{ListOptions: github.ListOptions{PerPage: s.perpage}})
	if err != nil {
		log.Printf("error at getting starred for %s: %s\n", owner, err)
	}
	return repos
}
func (s *StarLink) stargazers(owner, repo string) []github.User {
	users, _, err := s.client.Activity.ListStargazers(owner, repo, &github.ListOptions{PerPage: s.perpage})
	if err != nil {
		log.Printf("error at getting stargazers for %s/%s: %s\n", owner, repo, err)
	}
	return users
}

func (s *StarLink) suggest(owner, repo string, cbPerUser func()) []string {
	count := map[string]int{}
	repos := map[string]*github.Repository{}

	for _, user := range s.stargazers(owner, repo) {
		for _, repo := range s.starred(*user.Login) {
			count[*repo.FullName]++
			repos[*repo.FullName] = &repo
		}
		cbPerUser()
	}

	count2repo := map[int][]string{}
	for repo, c := range count {
		count2repo[c] = append(count2repo[c], repo)
	}
	cc := map[int]struct{}{}
	for _, c := range count {
		cc[c] = struct{}{}
	}
	counts := []int{}
	for c := range cc {
		counts = append(counts, c)
	}
	names := []string{}
	sort.Sort(sort.Reverse(sort.IntSlice(counts)))
	for _, c := range counts {
		for _, r := range count2repo[c] {
			names = append(names, r)
		}
	}
	return names
}
