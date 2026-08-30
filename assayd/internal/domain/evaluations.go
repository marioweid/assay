package domain

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	secretcrypto "github.com/marioweid/assay/assayd/internal/crypto"
	"github.com/marioweid/assay/assayd/internal/id"

	"github.com/google/uuid"
)

const (
	defaultPageLimit = 100
	maxPageLimit     = 500
	defaultThreshold = 0.5
)

// EvaluationService owns offline dataset, scorer configuration, and run workflows.
type EvaluationService struct {
	repository     EvaluationRepository
	cipher         *secretcrypto.Cipher
	jobMaxAttempts int
}

// NewEvaluationService constructs the offline evaluation workflow service.
func NewEvaluationService(
	repository EvaluationRepository,
	cipher *secretcrypto.Cipher,
	jobMaxAttempts int,
) *EvaluationService {
	return &EvaluationService{
		repository: repository, cipher: cipher, jobMaxAttempts: jobMaxAttempts,
	}
}

// CreateDataset validates and persists a dataset.
func (s *EvaluationService) CreateDataset(
	ctx context.Context,
	input CreateDatasetInput,
) (Dataset, error) {
	name, err := requiredValue("dataset name", input.Name)
	if err != nil {
		return Dataset{}, err
	}
	if _, err := s.repository.GetApplication(ctx, input.ApplicationID); err != nil {
		return Dataset{}, fmt.Errorf("create dataset: %w", err)
	}
	datasetID, err := id.New()
	if err != nil {
		return Dataset{}, fmt.Errorf("generate dataset ID: %w", err)
	}
	dataset, err := s.repository.CreateDataset(ctx, Dataset{
		ID: datasetID, ApplicationID: input.ApplicationID,
		Name: name, Description: trimmedOptional(input.Description),
	})
	if err != nil {
		return Dataset{}, fmt.Errorf("create dataset: %w", err)
	}
	return dataset, nil
}

// ListDatasets returns a cursor-paginated dataset page.
func (s *EvaluationService) ListDatasets(
	ctx context.Context,
	query DatasetQuery,
) (DatasetPage, error) {
	if err := normalizePageQuery(&query.PageQuery); err != nil {
		return DatasetPage{}, err
	}
	pageSize := query.Limit
	query.Limit++
	items, err := s.repository.ListDatasets(ctx, query)
	if err != nil {
		return DatasetPage{}, fmt.Errorf("list datasets: %w", err)
	}
	page := DatasetPage{Items: items}
	if len(items) > pageSize {
		page.Items = items[:pageSize]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &PageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// GetDataset returns a dataset by ID.
func (s *EvaluationService) GetDataset(ctx context.Context, datasetID uuid.UUID) (Dataset, error) {
	dataset, err := s.repository.GetDataset(ctx, datasetID)
	if err != nil {
		return Dataset{}, fmt.Errorf("get dataset %s: %w", datasetID, err)
	}
	return dataset, nil
}

// DeleteDataset removes a dataset and its dependent records.
func (s *EvaluationService) DeleteDataset(ctx context.Context, datasetID uuid.UUID) error {
	if err := s.repository.DeleteDataset(ctx, datasetID); err != nil {
		return fmt.Errorf("delete dataset %s: %w", datasetID, err)
	}
	return nil
}

// CreateDatasetItems validates and atomically persists evaluation cases.
func (s *EvaluationService) CreateDatasetItems(
	ctx context.Context,
	datasetID uuid.UUID,
	inputs []CreateDatasetItemInput,
) ([]DatasetItem, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("create dataset items: %w: no items supplied", ErrInvalid)
	}
	if _, err := s.repository.GetDataset(ctx, datasetID); err != nil {
		return nil, fmt.Errorf("create dataset items: %w", err)
	}
	items := make([]DatasetItem, 0, len(inputs))
	for index, input := range inputs {
		item, err := newDatasetItem(datasetID, input)
		if err != nil {
			return nil, fmt.Errorf("create dataset item %d: %w", index, err)
		}
		items = append(items, item)
	}
	items, err := s.repository.CreateDatasetItems(ctx, datasetID, items)
	if err != nil {
		return nil, fmt.Errorf("create dataset items: %w", err)
	}
	return items, nil
}

func newDatasetItem(datasetID uuid.UUID, input CreateDatasetItemInput) (DatasetItem, error) {
	question, ok := input.Input["question"].(string)
	question = strings.TrimSpace(question)
	if !ok || question == "" {
		return DatasetItem{}, fmt.Errorf("question: %w: must be a non-blank string", ErrInvalid)
	}
	output := strings.TrimSpace(input.Output)
	if output == "" {
		return DatasetItem{}, fmt.Errorf("output: %w: must not be blank", ErrInvalid)
	}
	context, err := normalizedChunks(input.Context)
	if err != nil {
		return DatasetItem{}, err
	}
	itemID, err := id.New()
	if err != nil {
		return DatasetItem{}, fmt.Errorf("generate dataset item ID: %w", err)
	}
	input.Input["question"] = question
	metadata := input.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	return DatasetItem{
		ID: itemID, DatasetID: datasetID, ExternalID: trimmedOptional(input.ExternalID),
		Input: input.Input, Output: &output, ExpectedOutput: input.ExpectedOutput,
		Context: context, Metadata: metadata,
	}, nil
}

// ListDatasetItems returns one cursor-paginated dataset item page.
func (s *EvaluationService) ListDatasetItems(
	ctx context.Context,
	datasetID uuid.UUID,
	query PageQuery,
) (DatasetItemPage, error) {
	if _, err := s.repository.GetDataset(ctx, datasetID); err != nil {
		return DatasetItemPage{}, fmt.Errorf("list dataset items: %w", err)
	}
	if err := normalizePageQuery(&query); err != nil {
		return DatasetItemPage{}, err
	}
	pageSize := query.Limit
	query.Limit++
	items, err := s.repository.ListDatasetItems(ctx, datasetID, query)
	if err != nil {
		return DatasetItemPage{}, fmt.Errorf("list dataset items: %w", err)
	}
	page := DatasetItemPage{Items: items}
	if len(items) > pageSize {
		page.Items = items[:pageSize]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &PageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func normalizePageQuery(query *PageQuery) error {
	if query.Limit == 0 {
		query.Limit = defaultPageLimit
	}
	if query.Limit < 1 || query.Limit > maxPageLimit {
		return fmt.Errorf("list page: %w: limit must be between 1 and 500", ErrInvalid)
	}
	if query.Cursor != nil && (query.Cursor.CreatedAt.IsZero() || query.Cursor.ID == uuid.Nil) {
		return fmt.Errorf("list page: %w: invalid cursor", ErrInvalid)
	}
	return nil
}

// ListScorerConfigs returns effective built-in configurations and overrides.
func (s *EvaluationService) ListScorerConfigs(
	ctx context.Context,
	applicationID uuid.UUID,
) ([]ScorerConfig, error) {
	if _, err := s.repository.GetApplication(ctx, applicationID); err != nil {
		return nil, fmt.Errorf("list scorer configs: %w", err)
	}
	persisted, err := s.repository.ListScorerConfigs(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list scorer configs: %w", err)
	}
	configs := effectiveConfigMap(applicationID, persisted)
	return []ScorerConfig{configs[ScorerCorrectness], configs[ScorerGroundedness]}, nil
}

// PutScorerConfig validates and persists one scorer override.
func (s *EvaluationService) PutScorerConfig(
	ctx context.Context,
	applicationID uuid.UUID,
	scorer string,
	input PutScorerConfigInput,
) (ScorerConfig, error) {
	base, err := s.scorerConfigBase(ctx, applicationID, scorer)
	if err != nil {
		return ScorerConfig{}, err
	}
	config, err := s.mergeScorerConfig(base, input)
	if err != nil {
		return ScorerConfig{}, err
	}
	config.ID, err = scorerConfigID(config.ID)
	if err != nil {
		return ScorerConfig{}, err
	}
	config.Persisted = true
	config, err = s.repository.UpsertScorerConfig(ctx, config)
	if err != nil {
		return ScorerConfig{}, fmt.Errorf("put scorer config: %w", err)
	}
	config.Persisted = true
	return config, nil
}

func (s *EvaluationService) scorerConfigBase(
	ctx context.Context,
	applicationID uuid.UUID,
	scorer string,
) (ScorerConfig, error) {
	if _, err := s.repository.GetApplication(ctx, applicationID); err != nil {
		return ScorerConfig{}, fmt.Errorf("put scorer config: %w", err)
	}
	base, found := defaultScorerConfig(applicationID, scorer)
	if !found {
		return ScorerConfig{}, fmt.Errorf(
			"put scorer config: %w: unknown scorer %q", ErrInvalid, scorer,
		)
	}
	persisted, err := s.repository.ListScorerConfigs(ctx, applicationID)
	if err != nil {
		return ScorerConfig{}, fmt.Errorf("put scorer config: %w", err)
	}
	for _, existing := range persisted {
		if existing.Scorer == scorer {
			if existing.JudgeConfig != nil && len(existing.JudgeConfig.APIKeyCiphertext) > 0 {
				base.JudgeConfig = &JudgeConfig{
					APIKeyCiphertext: existing.JudgeConfig.APIKeyCiphertext,
				}
			}
			return base, nil
		}
	}
	return base, nil
}

func scorerConfigID(current uuid.UUID) (uuid.UUID, error) {
	if current != uuid.Nil {
		return current, nil
	}
	configID, err := id.New()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate scorer config ID: %w", err)
	}
	return configID, nil
}

func (s *EvaluationService) mergeScorerConfig(
	config ScorerConfig,
	input PutScorerConfigInput,
) (ScorerConfig, error) {
	if input.Enabled != nil {
		config.Enabled = *input.Enabled
	}
	if input.Threshold != nil {
		if *input.Threshold < 0 || *input.Threshold > 1 {
			return ScorerConfig{}, fmt.Errorf("scorer threshold: %w: must be between 0 and 1", ErrInvalid)
		}
		config.Threshold = *input.Threshold
	}
	if input.PromptTemplateID != nil {
		config.PromptTemplateID = strings.TrimSpace(*input.PromptTemplateID)
	}
	if !validPrompt(config.Scorer, config.PromptTemplateID) {
		return ScorerConfig{}, fmt.Errorf("scorer prompt: %w: invalid prompt ID", ErrInvalid)
	}
	judge, err := s.scorerJudgeConfig(input.JudgeConfig, config.JudgeConfig)
	if err != nil {
		return ScorerConfig{}, err
	}
	config.JudgeConfig = judge
	return config, nil
}

func (s *EvaluationService) scorerJudgeConfig(
	input *JudgeConfigInput,
	current *JudgeConfig,
) (*JudgeConfig, error) {
	if input == nil {
		return current, nil
	}
	judge := &JudgeConfig{
		BaseURL: strings.TrimSpace(input.BaseURL),
		Model:   strings.TrimSpace(input.Model),
	}
	if err := validateHTTPURL("scorer judge base URL", judge.BaseURL, true); err != nil {
		return nil, err
	}
	ciphertext, err := s.scorerAPIKey(input.APIKey, current)
	if err != nil {
		return nil, err
	}
	judge.APIKeyCiphertext = ciphertext
	if judge.BaseURL == "" && judge.Model == "" && len(judge.APIKeyCiphertext) == 0 {
		return nil, nil
	}
	return judge, nil
}

func (s *EvaluationService) scorerAPIKey(apiKey *string, current *JudgeConfig) ([]byte, error) {
	if apiKey == nil {
		if current == nil {
			return nil, nil
		}
		return current.APIKeyCiphertext, nil
	}
	if *apiKey == "" {
		return nil, nil
	}
	ciphertext, err := s.cipher.Encrypt([]byte(*apiKey))
	if err != nil {
		return nil, fmt.Errorf("encrypt scorer judge API key: %w", err)
	}
	return ciphertext, nil
}

func validPrompt(scorer string, prompt string) bool {
	return scorer == ScorerGroundedness && prompt == GroundednessPromptV1 ||
		scorer == ScorerCorrectness && prompt == CorrectnessPromptV1
}

func normalizedChunks(chunks []Chunk) ([]Chunk, error) {
	seen := make(map[string]struct{}, len(chunks))
	result := make([]Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		chunk.ID = strings.TrimSpace(chunk.ID)
		chunk.Text = strings.TrimSpace(chunk.Text)
		if chunk.ID == "" || chunk.Text == "" {
			return nil, fmt.Errorf("context chunk: %w: id and text must not be blank", ErrInvalid)
		}
		if _, found := seen[chunk.ID]; found {
			return nil, fmt.Errorf("context chunk: %w: duplicate id %q", ErrInvalid, chunk.ID)
		}
		seen[chunk.ID] = struct{}{}
		result = append(result, chunk)
	}
	return result, nil
}

// ResolveScorerConfigs merges scorer, project, and process judge settings.
func (s *EvaluationService) ResolveScorerConfigs(
	ctx context.Context,
	applicationID uuid.UUID,
	names []string,
	defaults JudgeDefaults,
) ([]ResolvedScorerConfig, error) {
	application, err := s.repository.GetApplication(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("resolve scorer application: %w", err)
	}
	project, err := s.repository.GetProject(ctx, application.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("resolve scorer project: %w", err)
	}
	persisted, err := s.repository.ListScorerConfigs(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list scorer configs: %w", err)
	}
	configs := effectiveConfigMap(applicationID, persisted)
	return s.resolveNamedConfigs(names, configs, project.JudgeConfig, defaults)
}

func (s *EvaluationService) resolveNamedConfigs(
	names []string,
	configs map[string]ScorerConfig,
	project *JudgeConfig,
	defaults JudgeDefaults,
) ([]ResolvedScorerConfig, error) {
	seen := make(map[string]struct{}, len(names))
	resolved := make([]ResolvedScorerConfig, 0, len(names))
	for _, name := range names {
		if _, found := seen[name]; found {
			return nil, fmt.Errorf("resolve scorers: %w: duplicate scorer %q", ErrInvalid, name)
		}
		config, found := configs[name]
		if !found || !config.Enabled {
			return nil, fmt.Errorf("resolve scorers: %w: scorer %q is unavailable", ErrInvalid, name)
		}
		item, err := s.resolveConfig(config, project, defaults)
		if err != nil {
			return nil, err
		}
		seen[name] = struct{}{}
		resolved = append(resolved, item)
	}
	return resolved, nil
}

func (s *EvaluationService) resolveConfig(
	config ScorerConfig,
	project *JudgeConfig,
	defaults JudgeDefaults,
) (ResolvedScorerConfig, error) {
	judge := ResolvedJudgeConfig{
		BaseURL: defaults.BaseURL,
		Model:   defaults.Model,
		APIKey:  defaults.APIKey,
	}
	applyJudgeConfig(&judge, project)
	applyJudgeConfig(&judge, config.JudgeConfig)
	secret := selectedCiphertext(config.JudgeConfig, project)
	if len(secret) > 0 {
		plaintext, err := s.cipher.Decrypt(secret)
		if err != nil {
			return ResolvedScorerConfig{}, fmt.Errorf("decrypt scorer judge API key: %w", err)
		}
		judge.APIKey = string(plaintext)
	}
	if strings.TrimSpace(judge.BaseURL) == "" || strings.TrimSpace(judge.Model) == "" {
		return ResolvedScorerConfig{}, fmt.Errorf(
			"resolve scorer %q: %w: judge URL and model required",
			config.Scorer, ErrInvalid,
		)
	}
	parsed, err := url.Parse(judge.BaseURL)
	if err != nil || parsed.Hostname() == "" {
		return ResolvedScorerConfig{}, fmt.Errorf(
			"resolve scorer %q: %w: invalid judge URL", config.Scorer, ErrInvalid,
		)
	}
	judge.Provider = strings.ToLower(parsed.Hostname())
	resolved := ResolvedScorerConfig{
		Scorer: config.Scorer, Threshold: config.Threshold,
		PromptTemplateID: config.PromptTemplateID, Judge: judge,
	}
	if config.Persisted {
		resolved.ConfigID = &config.ID
	}
	return resolved, nil
}

// CreateEvalRun atomically creates a run, item outcomes, and durable job.
func (s *EvaluationService) CreateEvalRun(
	ctx context.Context,
	input CreateEvalRunInput,
) (EvalRun, error) {
	name, err := requiredValue("eval run name", input.Name)
	if err != nil {
		return EvalRun{}, err
	}
	if input.Mode == "" {
		input.Mode = EvalModeScoreExisting
	}
	if input.Mode != EvalModeScoreExisting {
		return EvalRun{}, fmt.Errorf("create eval run: %w: unsupported mode %q", ErrInvalid, input.Mode)
	}
	if err := validateScorerNames(input.Scorers); err != nil {
		return EvalRun{}, err
	}
	if err := s.validateEnabledScorers(ctx, input.ApplicationID, input.Scorers); err != nil {
		return EvalRun{}, err
	}
	count, err := s.validateRunResources(ctx, input.ApplicationID, input.DatasetID)
	if err != nil {
		return EvalRun{}, err
	}
	return s.persistRun(ctx, input, name, count)
}

func (s *EvaluationService) validateEnabledScorers(
	ctx context.Context,
	applicationID uuid.UUID,
	names []string,
) error {
	persisted, err := s.repository.ListScorerConfigs(ctx, applicationID)
	if err != nil {
		return fmt.Errorf("create eval run: list scorer configs: %w", err)
	}
	configs := effectiveConfigMap(applicationID, persisted)
	for _, name := range names {
		if !configs[name].Enabled {
			return fmt.Errorf("create eval run: %w: scorer %q is disabled", ErrInvalid, name)
		}
	}
	return nil
}

func (s *EvaluationService) validateRunResources(
	ctx context.Context,
	applicationID uuid.UUID,
	datasetID uuid.UUID,
) (int, error) {
	application, err := s.repository.GetApplication(ctx, applicationID)
	if err != nil {
		return 0, fmt.Errorf("create eval run: %w", err)
	}
	dataset, err := s.repository.GetDataset(ctx, datasetID)
	if err != nil {
		return 0, fmt.Errorf("create eval run: %w", err)
	}
	if dataset.ApplicationID != application.ID {
		return 0, fmt.Errorf(
			"create eval run: %w: dataset belongs to another application", ErrInvalid,
		)
	}
	count, err := s.repository.CountDatasetItems(ctx, dataset.ID)
	if err != nil {
		return 0, fmt.Errorf("count eval run items: %w", err)
	}
	if count == 0 {
		return 0, fmt.Errorf("create eval run: %w: dataset is empty", ErrInvalid)
	}
	return count, nil
}

func (s *EvaluationService) persistRun(
	ctx context.Context,
	input CreateEvalRunInput,
	name string,
	count int,
) (EvalRun, error) {
	runID, err := id.New()
	if err != nil {
		return EvalRun{}, fmt.Errorf("generate eval run ID: %w", err)
	}
	jobID, err := id.New()
	if err != nil {
		return EvalRun{}, fmt.Errorf("generate eval run job ID: %w", err)
	}
	params := input.Params
	if params == nil {
		params = map[string]any{}
	}
	run := EvalRun{
		ID: runID, ApplicationID: input.ApplicationID, DatasetID: input.DatasetID,
		Name: name, Status: EvalStatusPending, Mode: EvalModeScoreExisting,
		Params: params, Scorers: input.Scorers, TotalItems: count,
	}
	job := Job{
		ID: jobID, Kind: "eval_run", Status: EvalStatusPending,
		EvalRunID: runID, MaxAttempts: s.jobMaxAttempts,
	}
	run, err = s.repository.CreateEvalRun(ctx, run, job)
	if err != nil {
		return EvalRun{}, fmt.Errorf("create eval run: %w", err)
	}
	return run, nil
}

func validateScorerNames(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("create eval run: %w: at least one scorer required", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, found := defaultScorerConfig(uuid.Nil, name); !found {
			return fmt.Errorf("create eval run: %w: unsupported scorer %q", ErrInvalid, name)
		}
		if _, found := seen[name]; found {
			return fmt.Errorf("create eval run: %w: duplicate scorer %q", ErrInvalid, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// ListEvalRuns returns a filtered cursor-paginated run page.
func (s *EvaluationService) ListEvalRuns(
	ctx context.Context,
	query EvalRunQuery,
) (EvalRunPage, error) {
	if err := normalizePageQuery(&query.PageQuery); err != nil {
		return EvalRunPage{}, err
	}
	if query.Status != "" && !validEvalStatus(query.Status) {
		return EvalRunPage{}, fmt.Errorf("list eval runs: %w: invalid status", ErrInvalid)
	}
	pageSize := query.Limit
	query.Limit++
	runs, err := s.repository.ListEvalRuns(ctx, query)
	if err != nil {
		return EvalRunPage{}, fmt.Errorf("list eval runs: %w", err)
	}
	page := EvalRunPage{Items: runs}
	if len(runs) > pageSize {
		page.Items = runs[:pageSize]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &PageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// GetEvalRun returns an evaluation run by ID.
func (s *EvaluationService) GetEvalRun(ctx context.Context, runID uuid.UUID) (EvalRun, error) {
	run, err := s.repository.GetEvalRun(ctx, runID)
	if err != nil {
		return EvalRun{}, fmt.Errorf("get eval run %s: %w", runID, err)
	}
	return run, nil
}

// ListEvalRunItems returns cursor-paginated item outcomes.
func (s *EvaluationService) ListEvalRunItems(
	ctx context.Context,
	runID uuid.UUID,
	query PageQuery,
) (EvalRunItemPage, error) {
	if _, err := s.repository.GetEvalRun(ctx, runID); err != nil {
		return EvalRunItemPage{}, fmt.Errorf("list eval run items: %w", err)
	}
	if err := normalizePageQuery(&query); err != nil {
		return EvalRunItemPage{}, err
	}
	pageSize := query.Limit
	query.Limit++
	items, err := s.repository.ListEvalRunItems(ctx, runID, query)
	if err != nil {
		return EvalRunItemPage{}, fmt.Errorf("list eval run items: %w", err)
	}
	page := EvalRunItemPage{Items: items}
	if len(items) > pageSize {
		page.Items = items[:pageSize]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &PageCursor{CreatedAt: last.CreatedAt, ID: last.DatasetItemID}
	}
	return page, nil
}

// ListEvalRunScores returns bigint-cursor-paginated scores.
func (s *EvaluationService) ListEvalRunScores(
	ctx context.Context,
	runID uuid.UUID,
	query ScoreQuery,
) (ScorePage, error) {
	if _, err := s.repository.GetEvalRun(ctx, runID); err != nil {
		return ScorePage{}, fmt.Errorf("list eval run scores: %w", err)
	}
	if query.Limit == 0 {
		query.Limit = defaultPageLimit
	}
	if query.Limit < 1 || query.Limit > maxPageLimit {
		return ScorePage{}, fmt.Errorf("list eval run scores: %w: invalid limit", ErrInvalid)
	}
	pageSize := query.Limit
	query.Limit++
	scores, err := s.repository.ListEvalRunScores(ctx, runID, query)
	if err != nil {
		return ScorePage{}, fmt.Errorf("list eval run scores: %w", err)
	}
	page := ScorePage{Items: scores}
	if len(scores) > pageSize {
		page.Items = scores[:pageSize]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &ScoreCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// CancelEvalRun cancels an active run and its pending work.
func (s *EvaluationService) CancelEvalRun(ctx context.Context, runID uuid.UUID) (EvalRun, error) {
	run, err := s.repository.CancelEvalRun(ctx, runID)
	if err != nil {
		return EvalRun{}, fmt.Errorf("cancel eval run %s: %w", runID, err)
	}
	return run, nil
}

func validEvalStatus(status string) bool {
	switch status {
	case EvalStatusPending, EvalStatusRunning, EvalStatusSucceeded,
		EvalStatusFailed, EvalStatusCanceled:
		return true
	default:
		return false
	}
}

func effectiveConfigMap(applicationID uuid.UUID, persisted []ScorerConfig) map[string]ScorerConfig {
	groundedness, _ := defaultScorerConfig(applicationID, ScorerGroundedness)
	correctness, _ := defaultScorerConfig(applicationID, ScorerCorrectness)
	configs := map[string]ScorerConfig{
		ScorerGroundedness: groundedness,
		ScorerCorrectness:  correctness,
	}
	for _, config := range persisted {
		config.Persisted = true
		configs[config.Scorer] = config
	}
	return configs
}

func defaultScorerConfig(applicationID uuid.UUID, scorer string) (ScorerConfig, bool) {
	prompt := ""
	switch scorer {
	case ScorerGroundedness:
		prompt = GroundednessPromptV1
	case ScorerCorrectness:
		prompt = CorrectnessPromptV1
	default:
		return ScorerConfig{}, false
	}
	return ScorerConfig{
		ApplicationID: applicationID, Scorer: scorer, Enabled: true,
		Threshold: defaultThreshold, PromptTemplateID: prompt,
	}, true
}

func applyJudgeConfig(target *ResolvedJudgeConfig, source *JudgeConfig) {
	if source == nil {
		return
	}
	if source.BaseURL != "" {
		target.BaseURL = source.BaseURL
	}
	if source.Model != "" {
		target.Model = source.Model
	}
}

func selectedCiphertext(config *JudgeConfig, project *JudgeConfig) []byte {
	if config != nil && len(config.APIKeyCiphertext) > 0 {
		return config.APIKeyCiphertext
	}
	if project != nil {
		return project.APIKeyCiphertext
	}
	return nil
}

func trimmedOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
