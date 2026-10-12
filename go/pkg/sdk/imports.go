package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Source and mapping configuration are provider-owned JSON; preserve them unchanged.
type CreateImportJobRequest struct {
	Name               string          `json:"name"`
	ProviderAuthID     string          `json:"provider_auth_id"`
	TargetTable        string          `json:"target_table"`
	Mode               string          `json:"mode"`
	SourceConfig       json.RawMessage `json:"source_config"`
	MappingConfig      json.RawMessage `json:"mapping_config,omitempty"`
	IDField            string          `json:"id_field,omitempty"`
	DocumentIDTemplate string          `json:"document_id_template,omitempty"`
	RunPolicy          string          `json:"run_policy"`
	ScheduleInterval   int             `json:"schedule_interval_seconds,omitempty"`
}

type ImportJob struct {
	ID                 string          `json:"id"`
	OrganizationID     string          `json:"organization_id"`
	CloudInstanceID    string          `json:"cloud_instance_id"`
	ProviderAuthID     string          `json:"provider_auth_id"`
	TableName          string          `json:"table_name"`
	Name               string          `json:"name"`
	Mode               string          `json:"mode"`
	SourceConfig       json.RawMessage `json:"source_config"`
	MappingConfig      json.RawMessage `json:"mapping_config"`
	IDField            string          `json:"id_field"`
	DocumentIDTemplate string          `json:"document_id_template"`
	RunPolicy          string          `json:"run_policy"`
	ScheduleInterval   *int            `json:"schedule_interval_seconds,omitempty"`
	NextRunAt          *time.Time      `json:"next_run_at,omitempty"`
	LastScheduledRunAt *time.Time      `json:"last_scheduled_enqueue_at,omitempty"`
	Status             string          `json:"status"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	LastRun            *ImportJobRun   `json:"last_run,omitempty"`
}

type ImportJobRun struct {
	ID           string         `json:"id"`
	ImportJobID  string         `json:"import_job_id"`
	Status       string         `json:"status"`
	Diagnostics  RunDiagnostics `json:"diagnostics"`
	ErrorMessage *string        `json:"error_message,omitempty"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type RunDiagnostics struct {
	FailedSourceCount      int             `json:"failed_source_count,omitempty"`
	RetrySourceCount       int             `json:"retry_source_count,omitempty"`
	ReportingError         string          `json:"reporting_error,omitempty"`
	DiagnosticCount        int             `json:"diagnostic_count,omitempty"`
	ErrorCount             int             `json:"error_count,omitempty"`
	Outcome                string          `json:"outcome,omitempty"`
	Phase                  string          `json:"phase,omitempty"`
	LastObservedProgressAt *time.Time      `json:"last_observed_progress_at,omitempty"`
	RetryOfRunID           string          `json:"retry_of_run_id,omitempty"`
	RetrySourceKeys        []string        `json:"retry_source_keys,omitempty"`
	Summary                RunSummary      `json:"summary"`
	Diagnostics            []RunDiagnostic `json:"diagnostics,omitempty"`
}

type RunSummary struct {
	ObjectsRead                   int  `json:"objects_read,omitempty"`
	SelectedFiles                 int  `json:"selected_files,omitempty"`
	SupportedFiles                int  `json:"supported_files,omitempty"`
	UnsupportedFiles              int  `json:"unsupported_files,omitempty"`
	MalformedFiles                int  `json:"malformed_files,omitempty"`
	NativeFiles                   int  `json:"native_files,omitempty"`
	SectionFiles                  int  `json:"section_files,omitempty"`
	EmptyFiles                    int  `json:"empty_files,omitempty"`
	MappedRecords                 int  `json:"mapped_records,omitempty"`
	LoadableRecords               int  `json:"loadable_records,omitempty"`
	LoadedRecords                 int  `json:"loaded_records,omitempty"`
	SourceRowsLoaded              int  `json:"source_rows_loaded,omitempty"`
	SkippedRecords                int  `json:"skipped_records,omitempty"`
	FilesWithDocumentUnits        int  `json:"files_with_document_units,omitempty"`
	FilesWithDocumentChunks       int  `json:"files_with_document_chunks"`
	ExpectedDocumentChunks        int  `json:"expected_document_chunks,omitempty"`
	FullTextIndexedChunks         int  `json:"full_text_indexed_chunks,omitempty"`
	VectorIndexedChunks           int  `json:"vector_indexed_chunks,omitempty"`
	ExpectedSectionRecords        int  `json:"expected_section_records,omitempty"`
	LoadedSectionRecords          int  `json:"loaded_section_records,omitempty"`
	FullTextIndexedSectionRecords int  `json:"full_text_indexed_section_records,omitempty"`
	VectorIndexedSectionRecords   int  `json:"vector_indexed_section_records,omitempty"`
	FilesWithSearchableContent    *int `json:"files_with_searchable_content,omitempty"`
	FilesWithVectorCoverage       int  `json:"files_with_vector_coverage,omitempty"`
	FilesFailedIndexing           int  `json:"files_failed_indexing,omitempty"`
	RecordsSeen                   int  `json:"records_seen,omitempty"`
	RecordsLoaded                 int  `json:"records_loaded,omitempty"`
	RecordsSkipped                int  `json:"records_skipped,omitempty"`
	BatchesConfirmed              int  `json:"batches_confirmed,omitempty"`
	ConfirmedInserted             int  `json:"confirmed_inserted,omitempty"`
}

type RunDiagnostic struct {
	Status string               `json:"status,omitempty"`
	Phase  string               `json:"phase,omitempty"`
	Level  string               `json:"level"`
	Field  string               `json:"field,omitempty"`
	Record int                  `json:"record,omitempty"`
	Path   string               `json:"path,omitempty"`
	Code   string               `json:"code"`
	Detail string               `json:"detail"`
	Source *RunDiagnosticSource `json:"source,omitempty"`
}

type RunDiagnosticSource struct {
	SourceKind  string `json:"source_kind,omitempty"`
	URL         string `json:"url,omitempty"`
	Revision    string `json:"revision,omitempty"`
	Bucket      string `json:"bucket,omitempty"`
	Key         string `json:"key,omitempty"`
	Filename    string `json:"filename,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
	ETag        string `json:"etag,omitempty"`
	ObjectIndex int    `json:"object_index,omitempty"`
	RecordIndex int    `json:"record_index,omitempty"`
	LineNumber  int    `json:"line_number,omitempty"`
}

type DocumentTableHealth struct {
	TableName                     string         `json:"table_name"`
	JobID                         string         `json:"job_id"`
	LastRunID                     string         `json:"last_run_id,omitempty"`
	LastRunOutcome                string         `json:"last_run_outcome,omitempty"`
	LastRunStatus                 string         `json:"last_run_status,omitempty"`
	SourceRowsLoaded              int            `json:"source_rows_loaded"`
	FilesWithDocumentUnits        int            `json:"files_with_document_units"`
	FilesWithDocumentChunks       int            `json:"files_with_document_chunks"`
	ExpectedDocumentChunks        int            `json:"expected_document_chunks"`
	FullTextIndexedChunks         int            `json:"full_text_indexed_chunks"`
	VectorIndexedChunks           int            `json:"vector_indexed_chunks"`
	FilesWithVectorCoverage       int            `json:"files_with_vector_coverage"`
	FilesFailedIndexing           int            `json:"files_failed_indexing"`
	NativeFiles                   int            `json:"native_files,omitempty"`
	SectionFiles                  int            `json:"section_files,omitempty"`
	EmptyFiles                    int            `json:"empty_files,omitempty"`
	ExpectedSectionRecords        int            `json:"expected_section_records,omitempty"`
	LoadedSectionRecords          int            `json:"loaded_section_records,omitempty"`
	FullTextIndexedSectionRecords int            `json:"full_text_indexed_section_records,omitempty"`
	VectorIndexedSectionRecords   int            `json:"vector_indexed_section_records,omitempty"`
	FilesWithSearchableContent    *int           `json:"files_with_searchable_content,omitempty"`
	KnownFailureCategories        map[string]int `json:"known_failure_categories,omitempty"`
	SemanticSearchAvailable       bool           `json:"semantic_search_available"`
	SemanticSearchPartial         bool           `json:"semantic_search_partial"`
	FullTextSearchAvailable       bool           `json:"full_text_search_available"`
	GeneratedFromDiagnostics      bool           `json:"generated_from_diagnostics"`
}

type ImportProviderCapability struct {
	Provider string   `json:"provider"`
	Modes    []string `json:"modes"`
}

type ImportFormatCapability struct {
	ID         string   `json:"id"`
	Extensions []string `json:"extensions"`
	Modes      []string `json:"modes"`
	MIMEType   string   `json:"mime_type"`
}

type ImportCapabilities struct {
	Providers          []ImportProviderCapability `json:"providers"`
	Formats            []ImportFormatCapability   `json:"formats"`
	RetryableFileCodes []string                   `json:"retryable_file_codes"`
}

func instanceAPIPath(org, instance string) string {
	return "/organizations/" + url.PathEscape(org) + "/cloud/instances/" + url.PathEscape(instance)
}

func importAPIPath(org, instance, job string) (string, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return "", err
	}
	path := instanceAPIPath(orgID.String(), instanceID.String()) + "/import-jobs"
	if job != "" {
		jobID, err := parseUUID(job, "job")
		if err != nil {
			return "", err
		}
		path += "/" + jobID.String()
	}
	return path, nil
}

// requestJSON uses the same HTTP transport and request editors as generated calls.
// Credential endpoints intentionally omit response bodies from errors: a server
// or proxy may echo secrets. Import errors retain their actionable diagnostics.
func (c *Client) requestJSON(ctx context.Context, method, path string, body, out any, sensitive bool) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			if sensitive {
				return errors.New("invalid credential request JSON")
			}
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.transport.Server, "/")+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for _, editor := range c.transport.RequestEditors {
		if err := editor(ctx, req); err != nil {
			return err
		}
	}
	resp, err := c.transport.Client.Do(req)
	if err != nil {
		if sensitive {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("credential API request failed")
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if sensitive {
			return &APIError{StatusCode: resp.StatusCode}
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return apiError(resp.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		if sensitive {
			return errors.New("invalid credential API response JSON")
		}
		return fmt.Errorf("invalid API response JSON: %w", err)
	}
	return nil
}

func (c *Client) ImportCapabilities(ctx context.Context, org, instance string) (*ImportCapabilities, error) {
	path, err := importAPIPath(org, instance, "")
	if err != nil {
		return nil, err
	}
	var result struct {
		Data ImportCapabilities `json:"data"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, path+"/capabilities", nil, &result, false); err != nil {
		return nil, err
	}
	return &result.Data, nil
}

func (c *Client) ImportJobs(ctx context.Context, org, instance string, includeArchived bool) ([]ImportJob, error) {
	path, err := importAPIPath(org, instance, "")
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []ImportJob `json:"data"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, path+"?include_archived="+strconv.FormatBool(includeArchived), nil, &result, false); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (c *Client) CreateImportJob(ctx context.Context, org, instance string, req CreateImportJobRequest) (*ImportJob, error) {
	path, err := importAPIPath(org, instance, "")
	if err != nil {
		return nil, err
	}
	var result ImportJob
	if err := c.requestJSON(ctx, http.MethodPost, path, req, &result, false); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ImportJobRuns(ctx context.Context, org, instance, job string, limit, offset int) ([]ImportJobRun, error) {
	path, err := importAPIPath(org, instance, job)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []ImportJobRun `json:"data"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, path+"/runs?limit="+strconv.Itoa(limit)+"&offset="+strconv.Itoa(offset), nil, &result, false); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (c *Client) ImportJobHealth(ctx context.Context, org, instance, job string) (*DocumentTableHealth, error) {
	path, err := importAPIPath(org, instance, job)
	if err != nil {
		return nil, err
	}
	var result DocumentTableHealth
	if err := c.requestJSON(ctx, http.MethodGet, path+"/health", nil, &result, false); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) runImportJob(ctx context.Context, org, instance, job, action string) (*ImportJobRun, error) {
	path, err := importAPIPath(org, instance, job)
	if err != nil {
		return nil, err
	}
	var result ImportJobRun
	if err := c.requestJSON(ctx, http.MethodPost, path+"/"+action, nil, &result, false); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RunImportJob(ctx context.Context, org, instance, job string) (*ImportJobRun, error) {
	return c.runImportJob(ctx, org, instance, job, "run")
}

func (c *Client) RetryFailedImportFiles(ctx context.Context, org, instance, job string) (*ImportJobRun, error) {
	return c.runImportJob(ctx, org, instance, job, "retry-failed")
}

func (c *Client) updateImportJob(ctx context.Context, org, instance, job, action string) (*ImportJob, error) {
	path, err := importAPIPath(org, instance, job)
	if err != nil {
		return nil, err
	}
	var result ImportJob
	if err := c.requestJSON(ctx, http.MethodPost, path+"/"+action, nil, &result, false); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) PauseImportJob(ctx context.Context, org, instance, job string) (*ImportJob, error) {
	return c.updateImportJob(ctx, org, instance, job, "pause")
}

func (c *Client) ResumeImportJob(ctx context.Context, org, instance, job string) (*ImportJob, error) {
	return c.updateImportJob(ctx, org, instance, job, "resume")
}

func (c *Client) ArchiveImportJob(ctx context.Context, org, instance, job string) (*ImportJob, error) {
	return c.updateImportJob(ctx, org, instance, job, "archive")
}

// ImportRunResult distinguishes durable terminal status from successful ingestion.
// Missing outcomes in historical runs are not sufficient evidence of success.
func ImportRunResult(run ImportJobRun) (terminal bool, err error) {
	switch run.Status {
	case "pending", "running":
		return false, nil
	case "complete":
		if run.Diagnostics.Outcome == "success" && run.Diagnostics.ReportingError == "" && run.Diagnostics.ErrorCount == 0 && (run.ErrorMessage == nil || *run.ErrorMessage == "") {
			return true, nil
		}
		outcome := run.Diagnostics.Outcome
		if outcome == "" {
			outcome = "unknown (missing diagnostics.outcome)"
		}
		return true, fmt.Errorf("import run %s completed with outcome %s; inspect run diagnostics", run.ID, outcome)
	case "failed", "error", "canceled", "cancelled":
		return true, fmt.Errorf("import run %s ended with status %s; inspect run diagnostics", run.ID, run.Status)
	default:
		return true, fmt.Errorf("import run %s has unknown status %q", run.ID, run.Status)
	}
}

// WaitImportJobRun waits only for runID, paging history rather than assuming the
// most recent run belongs to this caller. Cancellation stops polling, not the job.
// On timeout/cancellation the last observed run is returned with the error.
func (c *Client) WaitImportJobRun(ctx context.Context, org, instance, job, runID string, pollInterval time.Duration) (*ImportJobRun, error) {
	id, err := parseUUID(runID, "run")
	if err != nil {
		return nil, err
	}
	runID = id.String()
	jobID, err := parseUUID(job, "job")
	if err != nil {
		return nil, err
	}
	job = jobID.String()
	if pollInterval <= 0 {
		return nil, errors.New("poll interval must be positive")
	}
	var last *ImportJobRun
	for {
		found := false
		for offset := 0; ; offset += 100 {
			runs, err := c.ImportJobRuns(ctx, org, instance, job, 100, offset)
			if err != nil {
				return last, err
			}
			for i := range runs {
				if runs[i].ID != runID {
					continue
				}
				if runs[i].ImportJobID != "" && runs[i].ImportJobID != job {
					return last, errors.New("requested run belongs to another import job")
				}
				run := runs[i]
				last = &run
				found = true
				if terminal, err := ImportRunResult(run); terminal {
					return last, err
				}
				break
			}
			if found || len(runs) < 100 {
				break
			}
		}
		if !found {
			return last, fmt.Errorf("import run %s not found in job %s", runID, job)
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, ctx.Err()
		case <-timer.C:
		}
	}
}
