package bitbucket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	"github.com/vinisman/bbctl/internal/models"
)

// ============================================================================
// Repo IDs (the ScriptRunner API requires numeric projectId/repositoryId in the query)
// ============================================================================

type repoIDs struct {
	projectID    int64
	repositoryID int64
}

// ensureRepoIDs resolves numeric project/repository ids via core REST API.
// Results are cached by projectKey/slug for the lifetime of the call batch.
func (c *Client) ensureRepoIDs(repos []models.ExtendedRepository) (map[string]repoIDs, error) {
	ids := make(map[string]repoIDs, len(repos))
	var mu sync.Mutex
	var errs []string

	for i := range repos {
		key := repos[i].ProjectKey + "/" + repos[i].RepositorySlug
		q := url.Values{}
		status, body, err := c.doRaw(http.MethodGet,
			"/api/latest/projects/"+repos[i].ProjectKey+"/repos/"+repos[i].RepositorySlug,
			q, nil)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to fetch repo metadata for %s: %v", key, err))
			continue
		}
		if status != http.StatusOK {
			errs = append(errs, fmt.Sprintf("unexpected status %d fetching repo metadata for %s: %s",
				status, key, truncateBody(body)))
			continue
		}
		var meta struct {
			ID      int64 `json:"id"`
			Project struct {
				ID int64 `json:"id"`
			} `json:"project"`
		}
		if err := json.Unmarshal(body, &meta); err != nil {
			errs = append(errs, fmt.Sprintf("failed to parse repo metadata for %s: %v", key, err))
			continue
		}
		if meta.ID == 0 || meta.Project.ID == 0 {
			errs = append(errs, fmt.Sprintf("missing repo/project id in metadata for %s", key))
			continue
		}
		mu.Lock()
		ids[key] = repoIDs{projectID: meta.Project.ID, repositoryID: meta.ID}
		mu.Unlock()
	}

	if len(errs) > 0 {
		return ids, fmt.Errorf("errors resolving repo ids: %s", strings.Join(errs, "; "))
	}
	return ids, nil
}

func truncateBody(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// ============================================================================
// ScriptRunner merge checks
// ============================================================================

const scriptrunnerBase = "/scriptrunner-bitbucket/latest"

// GetMergeChecks fetches ScriptRunner merge checks concurrently for multiple repositories.
// ScriptRunner GET /mergechecks returns a JSON array directly.
func (c *Client) GetMergeChecks(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	ids, err := c.ensureRepoIDs(repos)
	if err != nil {
		// continue with the repos whose ids were resolved successfully
		if len(ids) == 0 {
			return nil, err
		}
		c.logger.Error("partial metadata resolution", "error", err)
	}

	type job struct {
		idx int
	}
	jobs := make(chan job, len(repos))
	type result struct {
		idx    int
		checks []models.ScriptRunnerMergeCheck
	}
	resultsCh := make(chan result, len(repos))
	errCh := make(chan error, len(repos))

	var wg sync.WaitGroup
	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers > len(repos) {
		maxWorkers = len(repos)
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			r := repos[j.idx]
			key := r.ProjectKey + "/" + r.RepositorySlug
			rid, ok := ids[key]
			if !ok {
				errCh <- fmt.Errorf("no repo ids for %s", key)
				continue
			}
			q := url.Values{}
			q.Set("repo", r.RepositorySlug)
			q.Set("repositoryId", strconv.FormatInt(rid.repositoryID, 10))
			q.Set("project", r.ProjectKey)
			q.Set("projectId", strconv.FormatInt(rid.projectID, 10))

			status, body, err := c.doRaw(http.MethodGet, scriptrunnerBase+"/mergechecks", q, nil)
			if err != nil {
				errCh <- fmt.Errorf("failed to get merge checks for %s: %w", key, err)
				continue
			}
			if status != http.StatusOK {
				errCh <- fmt.Errorf("unexpected status %d getting merge checks for %s: %s", status, key, truncateBody(body))
				continue
			}
			var checks []models.ScriptRunnerMergeCheck
			if err := json.Unmarshal(body, &checks); err != nil {
				errCh <- fmt.Errorf("failed to parse merge checks JSON for %s: %w", key, err)
				continue
			}
			resultsCh <- result{idx: j.idx, checks: checks}
		}
	}

	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}
	for i := range repos {
		jobs <- job{idx: i}
	}
	close(jobs)
	wg.Wait()
	close(resultsCh)
	close(errCh)

	out := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		out[i].ProjectKey = repos[i].ProjectKey
		out[i].RepositorySlug = repos[i].RepositorySlug
	}
	for res := range resultsCh {
		checks := res.checks
		out[res.idx].MergeChecks = &checks
	}
	var errs []string
	for e := range errCh {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("errors fetching merge checks: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// UpsertMergeChecks creates or updates merge checks (POST with an id inside the payload — ScriptRunner updates an existing hook).
// For update items the payload must contain the full set of fields: use
// MergeUpdateMergeChecks to merge current+target before the call.
func (c *Client) UpsertMergeChecks(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	ids, err := c.ensureRepoIDs(repos)
	if err != nil && len(ids) == 0 {
		return nil, err
	}

	type job struct {
		repoIndex int
		repo      models.ExtendedRepository
		check     models.ScriptRunnerMergeCheck
	}
	var total int
	for _, r := range repos {
		if r.MergeChecks != nil {
			total += len(*r.MergeChecks)
		}
	}
	if total == 0 {
		return []models.ExtendedRepository{}, nil
	}

	jobs := make(chan job, total)
	resultsCh := make(chan struct {
		repoIndex int
		check     models.ScriptRunnerMergeCheck
	}, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup
	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers > total {
		maxWorkers = total
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			key := j.repo.ProjectKey + "/" + j.repo.RepositorySlug
			rid, ok := ids[key]
			if !ok {
				errCh <- fmt.Errorf("no repo ids for %s", key)
				continue
			}
			canned, _ := j.check["canned-script"].(string)
			if canned == "" {
				errCh <- fmt.Errorf("merge check in %s is missing \"canned-script\" field", key)
				continue
			}
			q := url.Values{}
			q.Set("project", j.repo.ProjectKey)
			q.Set("projectId", strconv.FormatInt(rid.projectID, 10))
			q.Set("repository", j.repo.RepositorySlug)

			status, body, err := c.doRaw(http.MethodPost, scriptrunnerBase+"/mergechecks/"+canned, q, j.check)
			if err != nil {
				errCh <- fmt.Errorf("failed to upsert merge check in %s: %w", key, err)
				continue
			}
			if status >= 400 {
				errCh <- fmt.Errorf("upsert merge check in %s failed: HTTP %d: %s", key, status, truncateBody(body))
				continue
			}
			resultsCh <- struct {
				repoIndex int
				check     models.ScriptRunnerMergeCheck
			}{j.repoIndex, j.check}

			name, _ := j.check["name"].(string)
			c.logger.Info("Upserted merge check",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"canned", canned,
				"name", name)
		}
	}

	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}
	for i, r := range repos {
		if r.MergeChecks == nil {
			continue
		}
		for _, mc := range *r.MergeChecks {
			jobs <- job{repoIndex: i, repo: r, check: mc}
		}
	}
	close(jobs)
	wg.Wait()
	close(resultsCh)
	close(errCh)

	newRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		newRepos[i].ProjectKey = repos[i].ProjectKey
		newRepos[i].RepositorySlug = repos[i].RepositorySlug
		empty := []models.ScriptRunnerMergeCheck{}
		newRepos[i].MergeChecks = &empty
	}
	for res := range resultsCh {
		*newRepos[res.repoIndex].MergeChecks = append(*newRepos[res.repoIndex].MergeChecks, res.check)
	}

	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
	}
	createdRepos := []models.ExtendedRepository{}
	for _, r := range newRepos {
		if len(*r.MergeChecks) > 0 {
			createdRepos = append(createdRepos, r)
		}
	}
	return createdRepos, firstErr
}

// DeleteMergeChecks deletes merge checks by id concurrently.
// query: project/repository (+repositoryId/projectId from the payload, if present).
func (c *Client) DeleteMergeChecks(repos []models.ExtendedRepository) error {
	type job struct {
		repo  models.ExtendedRepository
		check models.ScriptRunnerMergeCheck
	}
	var total int
	for _, r := range repos {
		if r.MergeChecks != nil {
			total += len(*r.MergeChecks)
		}
	}
	if total == 0 {
		return nil
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup
	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers > total {
		maxWorkers = total
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			key := j.repo.ProjectKey + "/" + j.repo.RepositorySlug
			id, ok := MergeCheckID(j.check)
			if !ok {
				errCh <- fmt.Errorf("merge check in %s is missing \"id\"", key)
				continue
			}
			q := url.Values{}
			q.Set("project", j.repo.ProjectKey)
			q.Set("repo", j.repo.RepositorySlug)
			if v, ok := j.check["FIELD_PER_REPO_ID"].(float64); ok {
				q.Set("repositoryId", strconv.FormatFloat(v, 'f', -1, 64))
			}
			if v, ok := j.check["perProjectId"].(float64); ok {
				q.Set("projectId", strconv.FormatFloat(v, 'f', -1, 64))
			}

			status, body, err := c.doRaw(http.MethodDelete,
				scriptrunnerBase+"/mergechecks/"+strconv.FormatInt(id, 10), q, nil)
			if err != nil {
				errCh <- fmt.Errorf("failed to delete merge check %d in %s: %w", id, key, err)
				continue
			}
			if status >= 400 && status != http.StatusNotFound {
				errCh <- fmt.Errorf("delete merge check %d in %s failed: HTTP %d: %s", id, key, status, truncateBody(body))
				continue
			}
			c.logger.Info("Deleted merge check (or was not present)",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", id)
		}
	}

	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}
	for _, r := range repos {
		if r.MergeChecks == nil {
			continue
		}
		for _, mc := range *r.MergeChecks {
			jobs <- job{repo: r, check: mc}
		}
	}
	close(jobs)
	wg.Wait()
	close(errCh)

	var errs []string
	for e := range errCh {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors deleting merge checks: %s", strings.Join(errs, "; "))
	}
	return nil
}

// MergeCheckID extracts the numeric id from the free-form structure.
func MergeCheckID(m models.ScriptRunnerMergeCheck) (int64, bool) {
	switch v := m["id"].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

// AreMergeChecksEqual compares merge checks by target keys only (target ⊆ current).
// The id field is not compared: it is an identifier, not configuration.
// Extra current keys are allowed: ScriptRunner may return computed fields.
func AreMergeChecksEqual(current, target models.ScriptRunnerMergeCheck) bool {
	return mapSubsetEqual(current, target, "id")
}

// mapSubsetEqual checks that every key-value pair from subset
// is present and equal in full, except keys listed in ignore.
func mapSubsetEqual(full, subset map[string]any, ignore ...string) bool {
	ignored := map[string]struct{}{}
	for _, k := range ignore {
		ignored[k] = struct{}{}
	}
	for k, tv := range subset {
		if _, skip := ignored[k]; skip {
			continue
		}
		cv, ok := full[k]
		if !ok {
			return false
		}
		if !jsonEqual(cv, tv) {
			return false
		}
	}
	return true
}

// jsonEqual compares values via normalized JSON
// (independent of number types and key order in maps).
func jsonEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	// Normalize key order via generic decode into maps
	var av, bv any
	if err := json.Unmarshal(ab, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(bb, &bv); err != nil {
		return false
	}
	return deepEqualNormalized(av, bv)
}

func deepEqualNormalized(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			bvv, ok := bv[k]
			if !ok || !deepEqualNormalized(v, bvv) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqualNormalized(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// MergeUpdateMergeChecks merges the current state with the target: target keys
// take priority, everything else is preserved from current (the id stays from current).
// This way the POST does not lose ScriptRunner-specific fields returned by GET
// but not set in the target.
func MergeUpdateMergeChecks(current, target models.ScriptRunnerMergeCheck) models.ScriptRunnerMergeCheck {
	merged := models.ScriptRunnerMergeCheck{}
	for k, v := range current {
		merged[k] = v
	}
	for k, v := range target {
		if k == "id" {
			continue
		}
		merged[k] = v
	}
	if id, ok := current["id"]; ok {
		merged["id"] = id
	}
	return merged
}

// ============================================================================
// Default tasks (core Bitbucket API: /rest/default-tasks/latest)
// ============================================================================

// GetDefaultTasks fetches default tasks concurrently for multiple repositories.
func (c *Client) GetDefaultTasks(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	type job struct {
		idx int
	}
	jobs := make(chan job, len(repos))
	type result struct {
		idx   int
		tasks []models.DefaultTask
	}
	resultsCh := make(chan result, len(repos))
	errCh := make(chan error, len(repos))

	var wg sync.WaitGroup
	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers > len(repos) {
		maxWorkers = len(repos)
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			r := repos[j.idx]
			key := r.ProjectKey + "/" + r.RepositorySlug
			q := url.Values{}
			status, body, err := c.doRaw(http.MethodGet,
				"/default-tasks/latest/projects/"+r.ProjectKey+"/repos/"+r.RepositorySlug+"/tasks",
				q, nil)
			if err != nil {
				errCh <- fmt.Errorf("failed to get default tasks for %s: %w", key, err)
				continue
			}
			if status != http.StatusOK {
				errCh <- fmt.Errorf("unexpected status %d getting default tasks for %s: %s", status, key, truncateBody(body))
				continue
			}
			var resp models.DefaultTasksResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				errCh <- fmt.Errorf("failed to parse default tasks JSON for %s: %w", key, err)
				continue
			}
			resultsCh <- result{idx: j.idx, tasks: resp.Values}
		}
	}

	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}
	for i := range repos {
		jobs <- job{idx: i}
	}
	close(jobs)
	wg.Wait()
	close(resultsCh)
	close(errCh)

	out := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		out[i].ProjectKey = repos[i].ProjectKey
		out[i].RepositorySlug = repos[i].RepositorySlug
	}
	for res := range resultsCh {
		tasks := res.tasks
		out[res.idx].DefaultTasks = &tasks
	}
	var errs []string
	for e := range errCh {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("errors fetching default tasks: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// CreateDefaultTasks creates default tasks concurrently.
func (c *Client) CreateDefaultTasks(repos []models.ExtendedRepository) ([]models.ExtendedRepository, error) {
	type job struct {
		repoIndex int
		repo      models.ExtendedRepository
		task      models.DefaultTask
	}
	var total int
	for _, r := range repos {
		if r.DefaultTasks != nil {
			total += len(*r.DefaultTasks)
		}
	}
	if total == 0 {
		return []models.ExtendedRepository{}, nil
	}

	jobs := make(chan job, total)
	resultsCh := make(chan struct {
		repoIndex int
		task      models.DefaultTask
	}, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup
	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers > total {
		maxWorkers = total
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			key := j.repo.ProjectKey + "/" + j.repo.RepositorySlug
			q := url.Values{}
			status, body, err := c.doRaw(http.MethodPost,
				"/default-tasks/latest/projects/"+j.repo.ProjectKey+"/repos/"+j.repo.RepositorySlug+"/tasks",
				q, j.task)
			if err != nil {
				errCh <- fmt.Errorf("failed to create default task in %s: %w", key, err)
				continue
			}
			if status >= 400 {
				errCh <- fmt.Errorf("create default task in %s failed: HTTP %d: %s", key, status, truncateBody(body))
				continue
			}
			resultsCh <- struct {
				repoIndex int
				task      models.DefaultTask
			}{j.repoIndex, j.task}
			desc, _ := j.task["description"].(string)
			c.logger.Info("Created default task",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"description", firstLine(desc))
		}
	}

	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}
	for i, r := range repos {
		if r.DefaultTasks == nil {
			continue
		}
		for _, t := range *r.DefaultTasks {
			jobs <- job{repoIndex: i, repo: r, task: t}
		}
	}
	close(jobs)
	wg.Wait()
	close(resultsCh)
	close(errCh)

	newRepos := make([]models.ExtendedRepository, len(repos))
	for i := range repos {
		newRepos[i].ProjectKey = repos[i].ProjectKey
		newRepos[i].RepositorySlug = repos[i].RepositorySlug
		empty := []models.DefaultTask{}
		newRepos[i].DefaultTasks = &empty
	}
	for res := range resultsCh {
		*newRepos[res.repoIndex].DefaultTasks = append(*newRepos[res.repoIndex].DefaultTasks, res.task)
	}

	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
	}
	createdRepos := []models.ExtendedRepository{}
	for _, r := range newRepos {
		if len(*r.DefaultTasks) > 0 {
			createdRepos = append(createdRepos, r)
		}
	}
	return createdRepos, firstErr
}

// DeleteDefaultTasks deletes default tasks by id concurrently.
// Supports both the full task format (id inside) and the simplified
// {"DefaultTaskId": ...} form.
func (c *Client) DeleteDefaultTasks(repos []models.ExtendedRepository) error {
	type job struct {
		repo models.ExtendedRepository
		task models.DefaultTask
	}
	var total int
	for _, r := range repos {
		if r.DefaultTasks != nil {
			total += len(*r.DefaultTasks)
		}
	}
	if total == 0 {
		return nil
	}

	jobs := make(chan job, total)
	errCh := make(chan error, total)

	var wg sync.WaitGroup
	maxWorkers := config.GlobalMaxWorkers
	if maxWorkers > total {
		maxWorkers = total
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			key := j.repo.ProjectKey + "/" + j.repo.RepositorySlug
			var id int64
			var ok bool
			if id, ok = DefaultTaskID(j.task); !ok {
				errCh <- fmt.Errorf("default task in %s is missing \"id\"", key)
				continue
			}
			q := url.Values{}
			status, body, err := c.doRaw(http.MethodDelete,
				"/default-tasks/latest/projects/"+j.repo.ProjectKey+"/repos/"+j.repo.RepositorySlug+"/tasks/"+strconv.FormatInt(id, 10),
				q, nil)
			if err != nil {
				errCh <- fmt.Errorf("failed to delete default task %d in %s: %w", id, key, err)
				continue
			}
			if status >= 400 && status != http.StatusNotFound {
				errCh <- fmt.Errorf("delete default task %d in %s failed: HTTP %d: %s", id, key, status, truncateBody(body))
				continue
			}
			c.logger.Info("Deleted default task (or was not present)",
				"project", j.repo.ProjectKey,
				"repo", j.repo.RepositorySlug,
				"id", id)
		}
	}

	wg.Add(maxWorkers)
	for range maxWorkers {
		go worker()
	}
	for _, r := range repos {
		if r.DefaultTasks == nil {
			continue
		}
		for _, t := range *r.DefaultTasks {
			jobs <- job{repo: r, task: t}
		}
	}
	close(jobs)
	wg.Wait()
	close(errCh)

	var errs []string
	for e := range errCh {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors deleting default tasks: %s", strings.Join(errs, "; "))
	}
	return nil
}

// DefaultTaskID extracts the numeric task id from the free-form structure.
func DefaultTaskID(m models.DefaultTask) (int64, bool) {
	switch v := m["id"].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

// AreDefaultTasksEqual compares default tasks by target keys (target ⊆ current), id excluded.
func AreDefaultTasksEqual(current, target models.DefaultTask) bool {
	return mapSubsetEqual(current, target, "id")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 80 {
		s = s[:80] + "..."
	}
	return s
}

// mergeUpdateDefaultTasks is not needed: default tasks do not support update,
// an idempotent apply uses delete+create.
