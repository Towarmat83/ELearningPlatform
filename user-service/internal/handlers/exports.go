package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/genesary/pupitre/user-service/internal/models"
	"github.com/genesary/pupitre/user-service/internal/repository"
)

// previewLimit is the maximum number of rows returned by ExportPreview.
const previewLimit = 10

// Virtual field ID constants used for cross-service enrichment detection.
const (
	exportFieldCourseTitle = "course_title"
	exportFieldPathTitle   = "path_title"
)

// exportRequest is the body accepted by ExportPreview and ExportDownload.
type exportRequest struct {
	Category string            `json:"category"`
	Fields   []string          `json:"fields"`
	Filters  map[string]string `json:"filters"`
}

// enrichSlugField maps each virtual export field ID to the SQL field that
// provides its lookup key for cross-service enrichment.
//
//nolint:gochecknoglobals // static read-only enrichment metadata
var enrichSlugField = map[string]string{
	exportFieldCourseTitle: "courseslug",
	exportFieldPathTitle:   "path_slug",
}

// titleEntry is a slug+title pair from course-service list responses.
type titleEntry struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// ExportCategories returns the list of available export categories.
func (s *State) ExportCategories(writer http.ResponseWriter, req *http.Request) {
	cats := s.Repos.Export.Categories()
	s.JSON(writer, http.StatusOK, map[string]any{"categories": cats})
}

// ExportPreview returns the first 10 rows and the total count.
func (s *State) ExportPreview(writer http.ResponseWriter, req *http.Request) {
	var body exportRequest

	decodeErr := decode(req, &body)
	if decodeErr != nil {
		s.Error(writer, http.StatusBadRequest, "Corps de requête invalide")

		return
	}

	if body.Category == "" {
		s.Error(writer, http.StatusBadRequest, "Catégorie requise")

		return
	}

	sqlFields, virtualFields := splitVirtualFields(body.Fields)
	fetchFields, helperFields := buildFetchFields(sqlFields, virtualFields)

	headers, rows, total, err := s.Repos.Export.FetchRows(req.Context(), body.Category, fetchFields, body.Filters, previewLimit)
	if err != nil {
		if strings.HasPrefix(err.Error(), "unknown") {
			s.Error(writer, http.StatusBadRequest, err.Error())

			return
		}

		zap.L().Error("export preview failed", zap.Error(err))
		s.Error(writer, http.StatusInternalServerError, "Erreur lors de la génération de l'aperçu")

		return
	}

	if len(virtualFields) > 0 {
		s.enrichRows(req.Context(), rows, virtualFields)
		rows = removeHelperFields(rows, helperFields)
		headers = body.Fields
	}

	s.JSON(writer, http.StatusOK, map[string]any{
		"headers":    headers,
		"rows":       rows,
		"totalCount": total,
	})
}

// ExportDownload streams the full dataset as UTF-8 CSV and logs the download.
func (s *State) ExportDownload(writer http.ResponseWriter, req *http.Request) {
	claims := s.claims(req)

	var body exportRequest

	decodeErr := decode(req, &body)
	if decodeErr != nil {
		s.Error(writer, http.StatusBadRequest, "Corps de requête invalide")

		return
	}

	if body.Category == "" {
		s.Error(writer, http.StatusBadRequest, "Catégorie requise")

		return
	}

	filename := fmt.Sprintf("%s-export-%s.csv", body.Category, time.Now().Format(time.DateOnly))

	writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	sqlFields, virtualFields := splitVirtualFields(body.Fields)

	var rowCount int

	var writeErr error

	if len(virtualFields) == 0 {
		rowCount, writeErr = s.Repos.Export.WriteCSV(req.Context(), writer, body.Category, body.Fields, body.Filters)
	} else {
		rowCount, writeErr = s.writeEnrichedCSV(req.Context(), writer, body.Category, body.Fields, sqlFields, virtualFields, body.Filters)
	}

	if writeErr != nil {
		zap.L().Error("export download failed", zap.Error(writeErr))

		return
	}

	fieldsJSON, _ := json.Marshal(body.Fields) //nolint:errchkjson // marshalling []string never fails

	logEntry := &models.ExportLog{
		UserID:    claims.Subject,
		UserEmail: claims.Email,
		Category:  body.Category,
		Fields:    string(fieldsJSON),
		RowCount:  rowCount,
	}

	logErr := s.Repos.Export.LogDownload(req.Context(), logEntry)
	if logErr != nil {
		zap.L().Warn("export audit log failed", zap.Error(logErr))
	}
}

// writeEnrichedCSV loads all rows, enriches virtual fields, then writes CSV.
// Called only when at least one virtual field is in the requested fields.
func (s *State) writeEnrichedCSV(
	ctx context.Context,
	writer io.Writer,
	category string,
	origFields []string,
	sqlFields []string,
	virtualFields []string,
	filters map[string]string,
) (int, error) {
	fetchFields, helperFields := buildFetchFields(sqlFields, virtualFields)

	_, rows, _, fetchErr := s.Repos.Export.FetchRows(ctx, category, fetchFields, filters, 0)
	if fetchErr != nil {
		return 0, fmt.Errorf("fetch rows: %w", fetchErr)
	}

	s.enrichRows(ctx, rows, virtualFields)
	rows = removeHelperFields(rows, helperFields)

	csvWriter := csv.NewWriter(writer)

	headerErr := csvWriter.Write(append([]string(nil), origFields...))
	if headerErr != nil {
		return 0, fmt.Errorf("write csv header: %w", headerErr)
	}

	for _, rowData := range rows {
		record := make([]string, 0, len(origFields))

		for _, fieldID := range origFields {
			record = append(record, rowData[fieldID])
		}

		rowErr := csvWriter.Write(record)
		if rowErr != nil {
			return 0, fmt.Errorf("write csv row: %w", rowErr)
		}
	}

	csvWriter.Flush()

	flushErr := csvWriter.Error()
	if flushErr != nil {
		return 0, fmt.Errorf("csv flush: %w", flushErr)
	}

	return len(rows), nil
}

// splitVirtualFields partitions fields into SQL fields (returned by the DB)
// and virtual fields (populated by cross-service enrichment).
//
//nolint:gocritic // unnamedResult conflicts with project nonamedreturns policy
func splitVirtualFields(fields []string) ([]string, []string) {
	var sqlFields, virtualFields []string

	for _, fieldID := range fields {
		if repository.IsVirtualExportField(fieldID) {
			virtualFields = append(virtualFields, fieldID)
		} else {
			sqlFields = append(sqlFields, fieldID)
		}
	}

	return sqlFields, virtualFields
}

// buildFetchFields returns the fields to pass to FetchRows: sqlFields plus
// any slug fields for virtual enrichment not already in sqlFields.
// helperFields lists extra slug fields added for enrichment purposes.
//
//nolint:gocritic // unnamedResult conflicts with project nonamedreturns policy
func buildFetchFields(sqlFields, virtualFields []string) ([]string, map[string]bool) {
	inSQL := make(map[string]bool, len(sqlFields))

	for _, fieldID := range sqlFields {
		inSQL[fieldID] = true
	}

	helperFields := make(map[string]bool)

	var extra []string

	for _, virtualField := range virtualFields {
		slugField, found := enrichSlugField[virtualField]
		if !found || inSQL[slugField] {
			continue
		}

		if !helperFields[slugField] {
			helperFields[slugField] = true
			extra = append(extra, slugField)
		}
	}

	fetchFields := make([]string, 0, len(sqlFields)+len(extra))
	fetchFields = append(fetchFields, sqlFields...)
	fetchFields = append(fetchFields, extra...)

	return fetchFields, helperFields
}

// removeHelperFields removes from each row the fields that were added as
// enrichment helpers but were not in the original field selection.
func removeHelperFields(rows []map[string]string, helperFields map[string]bool) []map[string]string {
	if len(helperFields) == 0 {
		return rows
	}

	for _, rowData := range rows {
		for fieldID := range helperFields {
			delete(rowData, fieldID)
		}
	}

	return rows
}

// enrichRows fills in virtual field values for every row by querying
// course-service for title maps and replacing the empty SQL placeholder values.
func (s *State) enrichRows(ctx context.Context, rows []map[string]string, virtualFields []string) {
	needCourse := false
	needPath := false

	for _, fieldID := range virtualFields {
		if fieldID == exportFieldCourseTitle {
			needCourse = true
		}

		if fieldID == exportFieldPathTitle {
			needPath = true
		}
	}

	var courseTitles map[string]string
	if needCourse {
		courseTitles, _ = s.fetchCourseTitles(ctx)
	}

	var pathTitles map[string]string
	if needPath {
		pathTitles, _ = s.fetchPathTitles(ctx)
	}

	for rowIdx := range rows {
		if needCourse {
			rows[rowIdx][exportFieldCourseTitle] = courseTitles[rows[rowIdx]["courseslug"]]
		}

		if needPath {
			rows[rowIdx][exportFieldPathTitle] = pathTitles[rows[rowIdx]["path_slug"]]
		}
	}
}

// fetchCourseTitles returns a slug→title map fetched from course-service.
func (s *State) fetchCourseTitles(ctx context.Context) (map[string]string, error) {
	return s.fetchTitleMap(ctx, "/api/courses", "courses")
}

// fetchPathTitles returns a path slug→title map fetched from course-service.
func (s *State) fetchPathTitles(ctx context.Context) (map[string]string, error) {
	return s.fetchTitleMap(ctx, "/api/paths", "paths")
}

// fetchTitleMap calls course-service at endpointPath, decodes the JSON array
// stored under payloadKey, and returns a slug→title map.
func (s *State) fetchTitleMap(ctx context.Context, endpointPath, payloadKey string) (map[string]string, error) {
	rawURL := s.Config.CourseServiceURL + endpointPath

	httpReq, buildErr := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if buildErr != nil {
		return nil, fmt.Errorf("build request for %s: %w", endpointPath, buildErr)
	}

	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return nil, fmt.Errorf("fetch %s: %w", endpointPath, doErr)
	}

	defer resp.Body.Close() //nolint:errcheck // closing response body never needs to be acted upon

	var raw map[string]json.RawMessage

	decodeErr := json.NewDecoder(resp.Body).Decode(&raw)
	if decodeErr != nil {
		return nil, fmt.Errorf("decode response from %s: %w", endpointPath, decodeErr)
	}

	var entries []titleEntry

	unmarshalErr := json.Unmarshal(raw[payloadKey], &entries)
	if unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", payloadKey, unmarshalErr)
	}

	titles := make(map[string]string, len(entries))

	for _, entry := range entries {
		titles[entry.Slug] = entry.Title
	}

	return titles, nil
}
