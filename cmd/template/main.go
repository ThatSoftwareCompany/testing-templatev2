package main

import (
	"flag"
	"fmt"
	"os"

	platformtemplate "github.com/ThatSoftwareCompany/template-go-api/internal/platform/template"
)

func main() {
	command := flag.String("command", "validate", "template command: validate")
	manifestPath := flag.String("manifest", ".template/manifest.json", "path to the template manifest")
	templateVersion := flag.String("template-version", "", "template version for provenance recording")
	templateCommit := flag.String("template-commit", "", "template commit for provenance recording")
	flag.Parse()

	switch *command {
	case "validate":
		manifest, err := platformtemplate.Load(*manifestPath)
		if err == nil {
			err = manifest.Validate()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "template manifest validation failed")
			os.Exit(1)
		}
		fmt.Println("template manifest is valid")
	case "record-provenance":
		if err := platformtemplate.RecordProvenance(*manifestPath, *templateVersion, *templateCommit); err != nil {
			fmt.Fprintln(os.Stderr, "template provenance recording failed")
			os.Exit(1)
		}
		fmt.Println("template provenance recorded")
	default:
		fmt.Fprintln(os.Stderr, "-command must be validate or record-provenance")
		os.Exit(2)
	}
}
