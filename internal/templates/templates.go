package templates

import (
	"embed"
	"text/template"
)

//go:embed *.tmpl
var templatesFS embed.FS

var (
	// RegistrationMarkdown is the template for registration results in markdown
	RegistrationMarkdown *template.Template

	// ValidationMarkdown is the template for validation results in markdown
	ValidationMarkdown *template.Template

	// BatchMarkdown is the template for batch results in markdown
	BatchMarkdown *template.Template

	// SystemInfoMarkdown is the template for system info in markdown
	SystemInfoMarkdown *template.Template
)

// Template helper functions
var funcMap = template.FuncMap{
	"add": func(a, b int) int {
		return a + b
	},
}

func init() {
	var err error

	// Parse all templates with helper functions
	RegistrationMarkdown, err = template.New("registration.tmpl").Funcs(funcMap).ParseFS(templatesFS, "registration.tmpl")
	if err != nil {
		panic("failed to parse registration template: " + err.Error())
	}

	ValidationMarkdown, err = template.New("validation.tmpl").Funcs(funcMap).ParseFS(templatesFS, "validation.tmpl")
	if err != nil {
		panic("failed to parse validation template: " + err.Error())
	}

	BatchMarkdown, err = template.New("batch.tmpl").Funcs(funcMap).ParseFS(templatesFS, "batch.tmpl")
	if err != nil {
		panic("failed to parse batch template: " + err.Error())
	}

	SystemInfoMarkdown, err = template.New("system_info.tmpl").Funcs(funcMap).ParseFS(templatesFS, "system_info.tmpl")
	if err != nil {
		panic("failed to parse system_info template: " + err.Error())
	}
}
