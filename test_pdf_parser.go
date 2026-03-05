package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"askflow/internal/parser"
)

func main() {
	// Read the PDF file
	data, err := ioutil.ReadFile("test.pdf")
	if err != nil {
		log.Fatalf("Failed to read test.pdf: %v", err)
	}

	fmt.Printf("PDF file size: %d bytes\n", len(data))
	fmt.Println("=" + string(make([]byte, 60)) + "=")

	// Create parser
	dp := &parser.DocumentParser{}

	// Parse the PDF
	result, err := dp.Parse(data, "pdf")
	if err != nil {
		log.Fatalf("Failed to parse PDF: %v", err)
	}

	// Display results
	fmt.Printf("Extracted text length: %d characters\n", len(result.Text))
	fmt.Printf("Number of images: %d\n", len(result.Images))
	fmt.Println("\n" + string(make([]byte, 60)))
	fmt.Println("Extracted Text:")
	fmt.Println(string(make([]byte, 60)))
	fmt.Println(result.Text)
	fmt.Println("\n" + string(make([]byte, 60)))

	// Check for garbled text
	garbledCount := 0
	replacementCount := 0
	for _, r := range result.Text {
		if r == '\uFFFD' {
			replacementCount++
			garbledCount++
		}
		if r >= 0x0080 && r <= 0x009F && r != '\n' && r != '\r' && r != '\t' {
			garbledCount++
		}
	}

	totalChars := len([]rune(result.Text))
	if totalChars > 0 {
		garbledPercent := float64(garbledCount) / float64(totalChars) * 100
		replacementPercent := float64(replacementCount) / float64(totalChars) * 100
		
		fmt.Printf("\nGarbled character analysis:\n")
		fmt.Printf("Total characters: %d\n", totalChars)
		fmt.Printf("Garbled characters: %d (%.2f%%)\n", garbledCount, garbledPercent)
		fmt.Printf("Replacement characters (�): %d (%.2f%%)\n", replacementCount, replacementPercent)
		
		if garbledPercent > 5.0 {
			fmt.Printf("\n⚠️  WARNING: Garbled text detected (%.2f%% > 5%% threshold)\n", garbledPercent)
			fmt.Println("This PDF should trigger OCR fallback in the system.")
		} else {
			fmt.Printf("\n✓ Text appears clean (%.2f%% < 5%% threshold)\n", garbledPercent)
		}
	}

	// Display metadata
	fmt.Println("\nMetadata:")
	for k, v := range result.Metadata {
		fmt.Printf("  %s: %s\n", k, v)
	}
}
