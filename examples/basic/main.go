// Example basic demonstrates a minimal mcpx server with a single tool.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/AKhilRaghav0/hamr"
)

// SearchInput defines the input schema for the search tool.
type SearchInput struct {
	Query      string `json:"query" desc:"the search query" required:"true"`
	MaxResults int    `json:"max_results" desc:"maximum results to return" default:"10"`
	Format     string `json:"format" desc:"output format" enum:"json,text,markdown" default:"text"`
}

// Search handles the search tool invocation.
func Search(ctx context.Context, input SearchInput) (string, error) {
	results := fmt.Sprintf("Search results for %q (max: %d, format: %s):\n", input.Query, input.MaxResults, input.Format)
	results += "1. Result one\n2. Result two\n3. Result three"
	return results, nil
}

// GreetInput defines input for the greet tool.
type GreetInput struct {
	Name     string `json:"name" desc:"person to greet"`
	Greeting string `json:"greeting" desc:"greeting to use" default:"Hello"`
}

// Greet handles the greet tool invocation.
func Greet(ctx context.Context, input GreetInput) (string, error) {
	return fmt.Sprintf("%s, %s! Welcome to hamr.", input.Greeting, input.Name), nil
}

// ReverseInput defines input for the reverse tool.
type ReverseInput struct {
	Text string `json:"text" desc:"text to reverse"`
}

// Reverse handles the reverse tool invocation.
func Reverse(ctx context.Context, input ReverseInput) (string, error) {
	runes := []rune(input.Text)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes), nil
}

// WordCountInput defines input for the word_count tool.
type WordCountInput struct {
	Text string `json:"text" desc:"text to count words in"`
}

// WordCount counts words in the input text.
func WordCount(ctx context.Context, input WordCountInput) (string, error) {
	words := strings.Fields(input.Text)
	return fmt.Sprintf("Word count: %d", len(words)), nil
}

func main() {
	s := hamr.New("demo-server", "1.0.0")

	s.Tool("search", "Search for information", Search)
	s.Tool("greet", "Greet a person by name", Greet)
	s.Tool("reverse", "Reverse a string", Reverse)
	s.Tool("word_count", "Count words in text", WordCount)

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
