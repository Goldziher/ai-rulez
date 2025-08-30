package errors

import (
	"fmt"
	"strings"
)

func ConfigNotFound(startDir string) *RichError {
	err := New(ErrorTypeConfigNotFound, "find config",
		fmt.Errorf("no configuration file found"))
	err.Path = startDir
	err.Context["search_dir"] = startDir
	err.Context["supported_names"] = []string{
		"ai-rulez.yaml", "ai-rulez.yml",
		".ai-rulez.yaml", ".ai-rulez.yml",
		"ai_rulez.yaml", "ai_rulez.yml",
		".ai_rulez.yaml", ".ai_rulez.yml",
	}
	err.Suggestions = []string{
		"Run 'ai-rulez init' to create a new configuration file",
		"Create one of the supported config files: ai-rulez.yaml, .ai-rulez.yaml",
		"Check if you're in the correct directory",
		"Use --config flag to specify the config file path explicitly",
	}
	return err
}

func ConfigLoad(filename string, underlying error) *RichError {
	err := New(ErrorTypeConfig, "load config", underlying)
	err.Path = filename
	err.Context["filename"] = filename
	return err
}

func ConfigParse(filename string, underlying error) *RichError {
	err := New(ErrorTypeConfigInvalid, "parse config", underlying).
		WithPath(filename).
		WithContext("filename", filename).
		WithSuggestion("Check the YAML syntax - ensure proper indentation").
		WithSuggestion("Validate your YAML at: https://www.yamllint.com/").
		WithSuggestion("Common issues: tabs instead of spaces, missing colons, incorrect indentation")

	if strings.Contains(underlying.Error(), "line") {
		err.WithContext("parse_error", underlying.Error()) //nolint:errcheck
	}

	return err
}

func SchemaValidation(filename string, validationErrors []string) *RichError {
	err := New(ErrorTypeConfigSchema, "validate schema",
		fmt.Errorf("configuration validation failed: %d errors", len(validationErrors)))
	err.Path = filename
	err.Context["filename"] = filename
	err.Context["errors"] = validationErrors
	err.Context["error_count"] = len(validationErrors)

	err.Suggestions = []string{
		"Check the YAML syntax using a YAML validator",
		"Ensure all required fields are present (metadata.name, outputs)",
		"Verify the structure matches the schema at: https://github.com/Goldziher/ai-rulez/blob/main/schema/ai-rules-v1.schema.json",
		"Run 'ai-rulez validate' for detailed validation output",
	}

	for _, vErr := range validationErrors {
		if strings.Contains(vErr, "metadata") {
			err.WithSuggestion("Ensure 'metadata' section exists with 'name' field") //nolint:errcheck
		}
		if strings.Contains(vErr, "outputs") {
			err.WithSuggestion("Add at least one output file in the 'outputs' section") //nolint:errcheck
		}
		if strings.Contains(vErr, "priority") {
			err.WithSuggestion("Priority must be a number between 1 and 20") //nolint:errcheck
		}
	}

	return err
}

func CircularInclude(includeChain []string) *RichError {
	err := New(ErrorTypeConfigCircular, "load includes",
		fmt.Errorf("circular include detected"))

	if len(includeChain) > 0 {
		err.Path = includeChain[len(includeChain)-1]
	}

	err.Context["include_chain"] = includeChain
	err.Context["circular_file"] = includeChain[len(includeChain)-1]

	err.Suggestions = []string{
		fmt.Sprintf("Remove the circular reference in %s", includeChain[len(includeChain)-1]),
		"Restructure your includes to avoid circular dependencies",
		"Use a different include strategy (e.g., separate common rules)",
		"Consider merging the circular files into a single file",
	}

	return err
}

func TemplateNotFound(name string, available []string) *RichError {
	err := New(ErrorTypeTemplateNotFound, "load template",
		fmt.Errorf("template '%s' not found", name))
	err.Context["requested"] = name
	err.Context["available"] = available
	err.Suggestions = []string{
		fmt.Sprintf("Use one of the available templates: %s", strings.Join(available, ", ")),
		"Use 'default' for the standard template",
		"Define a custom template inline using Go template syntax",
		"Reference an external template file using '@filename'",
	}
	return err
}

func TemplateParse(name string, underlying error) *RichError {
	err := New(ErrorTypeTemplateSyntax, "parse template", underlying).
		WithContext("template", name).
		WithSuggestion("Check the template syntax - common issues: unclosed '{{', invalid dot notation").
		WithSuggestion("Validate your template at: https://golang.org/pkg/text/template/").
		WithSuggestion("Use a built-in template as a reference: 'default' or 'documentation'")

	if strings.Contains(underlying.Error(), "line") {
		err.WithContext("parse_error", underlying.Error()) //nolint:errcheck
	}

	return err
}

func TemplateExecute(name string, underlying error) *RichError {
	err := New(ErrorTypeTemplate, "execute template", underlying).
		WithContext("template", name).
		WithSuggestion("Check that all template variables are available in the data").
		WithSuggestion("Common issue: accessing a field that doesn't exist").
		WithSuggestion("Use {{if}} checks for optional fields")

	return err
}

func FileRead(path string, underlying error) *RichError {
	err := New(ErrorTypeFileRead, "read file", underlying).
		WithPath(path).
		WithSuggestion("Check if the file exists: %s", path).
		WithSuggestion("Verify you have read permissions for the file").
		WithSuggestion("Ensure the path is correct and accessible")

	return err
}

func FileWrite(path string, underlying error) *RichError {
	err := New(ErrorTypeFileWrite, "write file", underlying).
		WithPath(path).
		WithSuggestion("Check if you have write permissions for: %s", path).
		WithSuggestion("Ensure the parent directory exists").
		WithSuggestion("Check available disk space")

	return err
}

func FilePermission(path string, operation string) *RichError {
	err := New(ErrorTypeFilePermission, operation,
		fmt.Errorf("permission denied")).
		WithPath(path).
		WithSuggestion("Check file permissions: try 'ls -la %s'", path).
		WithSuggestion("You may need to run with appropriate permissions").
		WithSuggestion("Ensure the file/directory is not locked by another process")

	return err
}

func ValidationRequired(field string, context string) *RichError {
	err := New(ErrorTypeValidationRequired, "validate field",
		fmt.Errorf("required field '%s' is missing", field)).
		WithContext("field", field).
		WithContext("context", context).
		WithSuggestion("Add the required field '%s' to your configuration", field).
		WithSuggestion("Check the documentation for the correct field structure")

	return err
}

func CommandArgument(arg string, value string, reason string) *RichError {
	err := New(ErrorTypeCommandArgument, "parse argument",
		fmt.Errorf("invalid argument '%s': %s", arg, reason)).
		WithContext("argument", arg).
		WithContext("value", value).
		WithContext("reason", reason).
		WithSuggestion("Check the command help: 'ai-rulez [command] --help'").
		WithSuggestion("Ensure the argument value is in the correct format")

	return err
}

func MCPError(operation string, underlying error) *RichError {
	err := New(ErrorTypeMCP, operation, underlying).
		WithSuggestion("Check the MCP connection and try again").
		WithSuggestion("Ensure the MCP server is running correctly").
		WithSuggestion("Verify the request format matches the MCP protocol")

	return err
}

func RemoteConfigFetch(url string, underlying error) *RichError {
	err := New(ErrorTypeRemote, "fetch remote config", underlying).
		WithPath(url).
		WithContext("url", url).
		WithSuggestion("Check if the URL is accessible: %s", url).
		WithSuggestion("Verify your network connection").
		WithSuggestion("Ensure the remote server is responding correctly").
		WithSuggestion("Check if authentication is required for this resource")

	if underlying != nil {
		errorMsg := underlying.Error()
		if strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "deadline exceeded") {
			err.WithSuggestion("The request timed out - try again or check server response time") //nolint:errcheck
		}
		if strings.Contains(errorMsg, "connection refused") {
			err.WithSuggestion("Connection refused - check if the server is running") //nolint:errcheck
		}
		if strings.Contains(errorMsg, "no such host") {
			err.WithSuggestion("DNS resolution failed - check the hostname") //nolint:errcheck
		}
	}

	return err
}

func RemoteConfigParse(url string, underlying error) *RichError {
	err := New(ErrorTypeRemote, "parse remote config", underlying).
		WithPath(url).
		WithContext("url", url).
		WithSuggestion("Check the YAML syntax in the remote file").
		WithSuggestion("Ensure the remote file follows ai-rulez configuration format").
		WithSuggestion("Verify the remote file content type is text/yaml or application/yaml")

	if underlying != nil && strings.Contains(underlying.Error(), "line") {
		err.WithContext("parse_error", underlying.Error()) //nolint:errcheck
	}

	return err
}

func RemoteSSRFError(url string, reason string) *RichError {
	err := New(ErrorTypeRemoteSSRF, "validate URL",
		fmt.Errorf("URL blocked for security reasons: %s", reason)).
		WithPath(url).
		WithContext("url", url).
		WithContext("block_reason", reason).
		WithSuggestion("Use a public URL (not localhost, private IPs, or metadata services)").
		WithSuggestion("Ensure the URL uses http:// or https:// scheme").
		WithSuggestion("Avoid URLs that resolve to private IP ranges or loopback addresses")

	if strings.Contains(reason, "metadata") {
		err.WithSuggestion("Metadata service endpoints are blocked for security") //nolint:errcheck
	}
	if strings.Contains(reason, "localhost") || strings.Contains(reason, "127.0.0.1") {
		err.WithSuggestion("localhost and loopback addresses are not allowed") //nolint:errcheck
	}
	if strings.Contains(reason, "private") || strings.Contains(reason, "192.168") || strings.Contains(reason, "10.0.0") {
		err.WithSuggestion("Private IP ranges (RFC 1918) are not accessible") //nolint:errcheck
	}

	return err
}

func RemoteNetworkError(url string, underlying error) *RichError {
	err := New(ErrorTypeRemoteNetwork, "network request", underlying).
		WithPath(url).
		WithContext("url", url).
		WithSuggestion("Check your network connectivity").
		WithSuggestion("Verify the URL is accessible from your location").
		WithSuggestion("Check if a proxy or firewall is blocking the request")

	if underlying != nil {
		errorMsg := underlying.Error()
		if strings.Contains(errorMsg, "timeout") {
			err.WithSuggestion("Request timed out - the server may be slow or overloaded") //nolint:errcheck
		}
		if strings.Contains(errorMsg, "connection refused") {
			err.WithSuggestion("Connection refused - the server may be down") //nolint:errcheck
		}
		if strings.Contains(errorMsg, "no route to host") {
			err.WithSuggestion("No route to host - check network routing") //nolint:errcheck
		}
	}

	return err
}

func RemoteHTTPError(url string, statusCode int, status string) *RichError {
	err := New(ErrorTypeRemoteHTTP, "HTTP request",
		fmt.Errorf("HTTP %d: %s", statusCode, status)).
		WithPath(url).
		WithContext("url", url).
		WithContext("status_code", statusCode).
		WithContext("status", status)

	switch statusCode {
	case 400:
		err.WithSuggestion("Bad Request (400) - check the URL format") //nolint:errcheck
	case 401:
		err.WithSuggestion("Unauthorized (401) - authentication may be required")  //nolint:errcheck
		err.WithSuggestion("Check if you need to provide API keys or credentials") //nolint:errcheck
	case 403:
		err.WithSuggestion("Forbidden (403) - you don't have permission to access this resource") //nolint:errcheck
		err.WithSuggestion("Check if the resource requires special permissions")                  //nolint:errcheck
	case 404:
		err.WithSuggestion("Not Found (404) - the resource doesn't exist at this URL") //nolint:errcheck
		err.WithSuggestion("Verify the URL path is correct")                           //nolint:errcheck
	case 429:
		err.WithSuggestion("Too Many Requests (429) - you're being rate limited")  //nolint:errcheck
		err.WithSuggestion("Wait before retrying or contact the service provider") //nolint:errcheck
	case 500:
		err.WithSuggestion("Internal Server Error (500) - the remote server has an issue") //nolint:errcheck
		err.WithSuggestion("Try again later or contact the service provider")              //nolint:errcheck
	case 502:
		err.WithSuggestion("Bad Gateway (502) - upstream server error")  //nolint:errcheck
		err.WithSuggestion("The service may be temporarily unavailable") //nolint:errcheck
	case 503:
		err.WithSuggestion("Service Unavailable (503) - the service is temporarily down") //nolint:errcheck
		err.WithSuggestion("Try again later")                                             //nolint:errcheck
	case 504:
		err.WithSuggestion("Gateway Timeout (504) - the request timed out")   //nolint:errcheck
		err.WithSuggestion("The upstream server is too slow or unresponsive") //nolint:errcheck
	default:
		if statusCode >= 400 && statusCode < 500 {
			err.WithSuggestion("Client error (%d) - check your request", statusCode) //nolint:errcheck
		} else if statusCode >= 500 {
			err.WithSuggestion("Server error (%d) - the remote service has an issue", statusCode) //nolint:errcheck
		}
	}

	return err
}

func RemoteTimeoutError(url string, timeout interface{}) *RichError {
	err := New(ErrorTypeRemoteTimeout, "request timeout",
		fmt.Errorf("request timed out")).
		WithPath(url).
		WithContext("url", url).
		WithContext("timeout", timeout).
		WithSuggestion("The request took too long to complete").
		WithSuggestion("Check if the remote server is responding slowly").
		WithSuggestion("Try again later when the server may be less busy").
		WithSuggestion("Consider using a local copy of the configuration if available")

	return err
}
