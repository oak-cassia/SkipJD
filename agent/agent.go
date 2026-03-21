package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"skipjd/agent/tools"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

func main() {
	readPageToolTest()

	ctx := context.Background()

	fmt.Printf("key: %s\n", os.Getenv("GOOGLE_API_KEY"))

	model, err := gemini.NewModel(ctx, "gemini-3.1-flash-lite-preview", &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	timeAgent, err := llmagent.New(llmagent.Config{
		Name:        "hello_agent",
		Model:       model,
		Description: "say hi",
		Instruction: "say hi",
		Tools:       []tool.Tool{},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(timeAgent),
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

func readPageToolTest() {
	browser := tools.NewBrowser()
	defer browser.MustClose()

	// https://www.gamejob.co.kr/Recruit/joblist?menucode=duty&duty=16
	// https://www.gamejob.co.kr/Recruit/joblist?menucode=duty&duty=3
	result, err := tools.ReadPage(browser, "https://careers.nexon.com/recruit?jobCategories=6&jobCategories=1&jobCategories=7&jobCategories=4&jobCategories=10")
	if err != nil {
		panic(err)
	}

	println(result.PageTitle)
	println(result.FinalURL)
	println(result.VisibleText)

	for _, link := range result.Links {
		println(link.Text, link.URL)
	}

	os.Exit(0)
}
