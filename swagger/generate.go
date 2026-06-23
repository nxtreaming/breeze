package swagger

import (
	"fmt"
	"strings"

	"github.com/goccy/go-json"
)

// Generate builds and returns the full OpenAPI 3.1 JSON document from all
// routes registered via RegisterRoute.
func Generate() []byte {
	mu.RLock()
	title := apiTitle
	version := apiVersion
	desc := apiDesc
	mu.RUnlock()

	if title == "" {
		title = "Breeze API"
	}
	if version == "" {
		version = "1.0.0"
	}

	paths := map[string]PathItem{}

	for _, entry := range allRoutes() {
		if _, ok := paths[entry.path]; !ok {
			paths[entry.path] = PathItem{}
		}

		op := buildOperation(entry)
		paths[entry.path][entry.method] = op
	}

	spec := OpenAPI{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:       title,
			Version:     version,
			Description: desc,
		},
		Paths: paths,
	}

	b, _ := json.MarshalIndent(spec, "", "  ")
	return b
}

// buildOperation converts a routeEntry into an OpenAPI Operation.
func buildOperation(entry routeEntry) Operation {
	doc := entry.doc
	op := Operation{
		Summary:     doc.Title,
		Description: doc.Description,
		Tags:        doc.Tags,
		Responses:   map[string]Response{},
	}

	// ── Parameters & request body ──────────────────────────────────────────
	for _, group := range doc.Input {
		switch group.Type {

		case InputParams:
			// Path parameters – reflect the struct fields as individual params
			schema := InferSchema(group.Fields)
			if schema != nil && schema.Properties != nil {
				for name, fieldSchema := range schema.Properties {
					op.Parameters = append(op.Parameters, Parameter{
						Name:        name,
						In:          "path",
						Required:    true,
						Description: fieldSchema.Description,
						Schema:      &Schema{Type: fieldSchema.Type, Format: fieldSchema.Format},
					})
				}
			}

		case InputQuery:
			// Query parameters – each struct field becomes a separate param
			schema := InferSchema(group.Fields)
			if schema != nil && schema.Properties != nil {
				for name, fieldSchema := range schema.Properties {
					required := false
					if schema.Required != nil {
						for _, r := range schema.Required {
							if r == name {
								required = true
								break
							}
						}
					}
					op.Parameters = append(op.Parameters, Parameter{
						Name:        name,
						In:          "query",
						Required:    required,
						Description: fieldSchema.Description,
						Schema:      &Schema{Type: fieldSchema.Type, Format: fieldSchema.Format},
					})
				}
			}

		case InputHeader:
			// Header parameters
			schema := InferSchema(group.Fields)
			if schema != nil && schema.Properties != nil {
				for name, fieldSchema := range schema.Properties {
					op.Parameters = append(op.Parameters, Parameter{
						Name:        name,
						In:          "header",
						Description: fieldSchema.Description,
						Schema:      &Schema{Type: fieldSchema.Type},
					})
				}
			}

		case InputBody:
			// Request body – the whole struct as a JSON object schema
			bodySchema := InferSchema(group.Fields)
			if bodySchema != nil {
				op.RequestBody = &RequestBody{
					Description: group.Description,
					Required:    group.Required,
					Content: map[string]MediaType{
						"application/json": {Schema: bodySchema},
					},
				}
			}
		}
	}

	// ── Response ────────────────────────────────────────────────────────────
	status := doc.OutputStatus
	if status == 0 {
		status = 200
	}
	statusStr := fmt.Sprintf("%d", status)

	respDesc := doc.OutputDescription
	if respDesc == "" {
		respDesc = httpStatusText(status)
	}

	resp := Response{Description: respDesc}
	if doc.Output != nil {
		outSchema := InferSchema(doc.Output)
		if outSchema != nil {
			resp.Content = map[string]MediaType{
				"application/json": {Schema: outSchema},
			}
		}
	}
	op.Responses[statusStr] = resp

	return op
}

// GenerateUI returns an HTML page that embeds Swagger UI pointing at jsonPath.
// jsonPath is the URL the browser will fetch the OpenAPI JSON from (e.g. "/swagger.json").
func GenerateUI(jsonPath string) []byte {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
  window.onload = function () {
    SwaggerUIBundle({
      url: "` + jsonPath + `",
      dom_id: "#swagger-ui",
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      layout: "BaseLayout",
      deepLinking: true,
    });
  };
</script>
</body>
</html>`
	return []byte(strings.TrimSpace(html))
}

// httpStatusText maps common status codes to their reason phrases.
func httpStatusText(code int) string {
	m := map[int]string{
		200: "OK",
		201: "Created",
		204: "No Content",
		400: "Bad Request",
		401: "Unauthorized",
		403: "Forbidden",
		404: "Not Found",
		422: "Unprocessable Entity",
		500: "Internal Server Error",
	}
	if t, ok := m[code]; ok {
		return t
	}
	return "Response"
}
