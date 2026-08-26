package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"github.com/floholz/paperparrot/internal/render"
	"github.com/floholz/paperparrot/internal/schema"
	"github.com/floholz/paperparrot/internal/theme"
	"github.com/floholz/paperparrot/internal/tmpl"
	"github.com/floholz/paperparrot/templates"
)

// Server bundles what the routes need.
type Server struct {
	Renderer *render.Chrome
	Version  string
}

// previewBody is the request of /preview and /preview.pdf: inline fields
// override the stored template (unsaved editor state).
type previewBody struct {
	Template *string         `json:"template"`
	HTML     *string         `json:"html"`
	CSS      *string         `json:"css"`
	Schema   json.RawMessage `json:"schema"`
	Theme    json.RawMessage `json:"theme"`
	Page     *string         `json:"page"`
	Locale   *string         `json:"locale"`
	Data     map[string]any  `json:"data"`
}

// RegisterRoutes mounts /api/pp/*.
func (s *Server) RegisterRoutes(e *core.ServeEvent) {
	e.Router.GET("/api/pp/status", func(re *core.RequestEvent) error {
		open, err := RegistrationOpen(re.App)
		if err != nil {
			return err
		}
		return re.JSON(http.StatusOK, map[string]any{
			"registration": open,
			"render":       s.Renderer.Available(),
			"version":      s.Version,
		})
	})

	g := e.Router.Group("/api/pp")
	g.Bind(apis.RequireAuth())
	g.GET("/fonts/builtin", func(re *core.RequestEvent) error {
		return re.JSON(http.StatusOK, theme.Families(theme.Builtin(), true))
	})
	g.GET("/starters", func(re *core.RequestEvent) error {
		all, err := templates.All()
		if err != nil {
			return err
		}
		return re.JSON(http.StatusOK, all)
	})
	g.POST("/starters/{id}", func(re *core.RequestEvent) error {
		st, err := templates.Get(re.Request.PathValue("id"))
		if err != nil {
			return re.NotFoundError("Unknown starter.", err)
		}
		rec, err := CreateFromStarter(re.App, re.Auth.Id, st)
		if err != nil {
			return err
		}
		return re.JSON(http.StatusOK, rec)
	})
	g.POST("/preview", func(re *core.RequestEvent) error {
		html, err := s.composePreview(re)
		if err != nil {
			return err
		}
		return re.HTML(http.StatusOK, html)
	})
	g.POST("/preview.pdf", func(re *core.RequestEvent) error {
		if !s.Renderer.Available() {
			return re.BadRequestError("PDF rendering is not available on this instance (no Chromium).", nil)
		}
		html, err := s.composePreview(re)
		if err != nil {
			return err
		}
		pdf, err := s.Renderer.PDF(re.Request.Context(), html)
		if err != nil {
			return re.InternalServerError("Render failed: "+err.Error(), err)
		}
		re.Response.Header().Set("Content-Disposition", `inline; filename="preview.pdf"`)
		return re.Blob(http.StatusOK, "application/pdf", pdf)
	})
	g.POST("/documents/{id}/render", s.renderDocument)
	g.POST("/documents/{id}/duplicate", s.duplicateDocument)
}

// composePreview builds the HTML for a preview request.
func (s *Server) composePreview(re *core.RequestEvent) (string, error) {
	var body previewBody
	if err := re.BindBody(&body); err != nil {
		return "", re.BadRequestError("Invalid request body.", err)
	}
	doc := tmpl.Doc{Theme: theme.Tokens{}, Page: "A4", Locale: "de-AT", Title: "Preview"}
	var assets tmpl.Assets
	if body.Template != nil && *body.Template != "" {
		tpl, err := s.ownedRecord(re, "templates", *body.Template)
		if err != nil {
			return "", err
		}
		doc, err = docFromRecord(re.App, tpl, re.Auth.Id)
		if err != nil {
			return "", re.BadRequestError(err.Error(), err)
		}
		doc.Title = tpl.GetString("name")
		if assets, err = templateAssets(re.App, tpl); err != nil {
			return "", err
		}
	}
	if body.HTML != nil {
		doc.HTML = *body.HTML
	}
	if body.CSS != nil {
		doc.CSS = *body.CSS
	}
	if body.Page != nil && *body.Page != "" {
		doc.Page = *body.Page
	}
	if body.Locale != nil && *body.Locale != "" {
		doc.Locale = *body.Locale
	}
	if len(body.Theme) > 0 && string(body.Theme) != "null" {
		tokens, err := theme.ParseTokens(body.Theme)
		if err != nil {
			return "", re.BadRequestError(err.Error(), err)
		}
		doc.Theme = tokens
		if doc.Fonts, err = userFonts(re.App, re.Auth.Id, tokens.Families()); err != nil {
			return "", err
		}
	}
	data := body.Data
	if len(body.Schema) > 0 && string(body.Schema) != "null" {
		sc, err := schema.Parse(body.Schema)
		if err != nil {
			return "", re.BadRequestError(err.Error(), err)
		}
		data = sc.ApplyDefaults(data, time.Now())
	}
	html, err := tmpl.Compose(doc, data, assets)
	if err != nil {
		return "", re.BadRequestError(err.Error(), err)
	}
	return html, nil
}

// renderDocument validates, composes, renders and stores an immutable render.
func (s *Server) renderDocument(re *core.RequestEvent) error {
	if !s.Renderer.Available() {
		return re.BadRequestError("PDF rendering is not available on this instance (no Chromium).", nil)
	}
	doc, err := s.ownedRecord(re, "documents", re.Request.PathValue("id"))
	if err != nil {
		return err
	}
	tpl, err := re.App.FindRecordById("templates", doc.GetString("template"))
	if err != nil {
		return re.BadRequestError("Template not found.", err)
	}
	sc, err := schema.Parse(rawField(tpl, "schema"))
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	data := jsonField(doc, "data")
	if errs := sc.Validate(data, true); len(errs) > 0 {
		return re.BadRequestError("The document is incomplete: "+errs.Error(), map[string]any{"errors": errs})
	}
	rev, err := findRevision(re.App, tpl)
	if err != nil {
		return err
	}
	d, err := docFromRecord(re.App, rev, re.Auth.Id)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	title := strings.TrimSpace(doc.GetString("title"))
	if title == "" {
		title = deriveTitle(tpl, data)
	}
	d.Title = title
	assets, err := templateAssets(re.App, tpl)
	if err != nil {
		return err
	}
	html, err := tmpl.Compose(d, data, assets)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	pdf, err := s.Renderer.PDF(re.Request.Context(), html)
	if err != nil {
		return re.InternalServerError("Render failed: "+err.Error(), err)
	}

	col, err := re.App.FindCollectionByNameOrId("renders")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("user", re.Auth.Id)
	rec.Set("document", doc.Id)
	rec.Set("revision", rev.Id)
	rec.Set("data", data)
	rec.Set("html", html)
	rec.Set("title", title)
	f, err := filesystem.NewFileFromBytes(pdf, sanitizeFilename(title)+".pdf")
	if err != nil {
		return err
	}
	rec.Set("pdf", f)
	if err := re.App.Save(rec); err != nil {
		return err
	}
	return re.JSON(http.StatusOK, rec)
}

// duplicateDocument copies a document: new sequences, fresh "today" dates.
func (s *Server) duplicateDocument(re *core.RequestEvent) error {
	src, err := s.ownedRecord(re, "documents", re.Request.PathValue("id"))
	if err != nil {
		return err
	}
	tpl, err := re.App.FindRecordById("templates", src.GetString("template"))
	if err != nil {
		return re.BadRequestError("Template not found.", err)
	}
	sc, err := schema.Parse(rawField(tpl, "schema"))
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	data := sc.ResetForDuplicate(jsonField(src, "data"), time.Now())
	rec := core.NewRecord(src.Collection())
	rec.Set("user", re.Auth.Id)
	rec.Set("template", tpl.Id)
	rec.Set("data", data)
	rec.Set("title", "") // derived by the create hook
	if err := re.App.Save(rec); err != nil {
		return err
	}
	return re.JSON(http.StatusOK, rec)
}

// ownedRecord loads a record and checks it belongs to the caller.
func (s *Server) ownedRecord(re *core.RequestEvent, collection, id string) (*core.Record, error) {
	rec, err := re.App.FindRecordById(collection, id)
	if err != nil || rec.GetString("user") != re.Auth.Id {
		return nil, re.NotFoundError("Not found.", err)
	}
	return rec, nil
}
