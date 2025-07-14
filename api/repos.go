package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/vinisman/bbctl/models"
	"github.com/vinisman/bbctl/utils"
)

type RepoService struct {
	cfg        utils.Config
	projectKey string
	client     *http.Client
}

func NewRepoService(cfg utils.Config, projectKey string) *RepoService {

	return &RepoService{cfg: cfg, projectKey: projectKey, client: http.DefaultClient}
}

// Create new repository
func (s *RepoService) Create(repo models.RepoEntity) {
	log := slog.With("repo", repo.Name)
	url := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos", s.cfg.BitbucketURL, s.projectKey)
	body := map[string]interface{}{
		"name":          repo.Name,
		"description":   repo.Description,
		"defaultBranch": repo.DefaultBranch,
		"scmId":         "git",
	}
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return
	}

	req, _ := http.NewRequest("POST", url, buf)
	req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Error("failed to create")
		log.Debug("details", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))
		return
	}

	log.Info("created")

}

// Update existing repository
func (s *RepoService) Update(repo models.RepoEntity) error {
	log := slog.With("repo", repo.Name)
	url := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s",
		s.cfg.BitbucketURL, s.projectKey, repo.Name)
	body := map[string]interface{}{
		"description":   repo.Description,
		"defaultBranch": repo.DefaultBranch,
	}
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		log.Error("A decode error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return err
	}

	req, _ := http.NewRequest("PUT", url, buf)
	req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		slog.Error("A connection error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("failed to update %q: HTTP %d", repo.Name, resp.StatusCode)
	}

	log.Info("updated")
	return nil
}

// Create or update repository
func (s *RepoService) CreateOrUpdate(repo models.RepoEntity) error {
	log := slog.With("repo", repo.Name)
	url := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s", s.cfg.BitbucketURL, s.projectKey, repo.Name)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)
	resp, err := s.client.Do(req)
	if err != nil {
		log.Error("A connection error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Debug("response", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))
		return s.Update(repo)
	}
	s.Create(repo)
	return nil
}

// Delete repository
func (s *RepoService) Delete(repo models.RepoEntity) error {
	log := slog.With("repo", repo.Name)
	url := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s",
		s.cfg.BitbucketURL, s.projectKey, repo.Name)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)

	resp, err := s.client.Do(req)
	if err != nil {
		log.Error("A connection error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Error("failed to create", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))
		return fmt.Errorf("failed to delete %q: HTTP %d", repo.Name, resp.StatusCode)
	}

	log.Info("deleted")
	return nil
}

// List repositories
func (s *RepoService) List() ([]models.RepoEntity, error) {
	var result []models.RepoEntity
	start := 0

	for {
		url := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos?limit=100&start=%d",
			s.cfg.BitbucketURL, s.projectKey, start)

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)
		req.Header.Set("Accept", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			slog.Error("A connection error occurred")
			slog.Debug("An error occurred", slog.String("error", err.Error()))
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			slog.Debug("response", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))
			return nil, fmt.Errorf("failed listing repos: status %d", resp.StatusCode)
		}

		var page models.RepoListResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			slog.Debug("An error occurred", slog.String("error", err.Error()))
			return nil, err
		}

		result = append(result, page.Values...)

		if page.IsLastPage {
			break
		}
		start = page.NextPageStart
	}

	return result, nil
}

// Get — получает данные по конкретной репе (slug) внутри проекта
func (s *RepoService) Get(repoSlug string) (*models.RepoEntity, error) {
	log := slog.With("repo", repoSlug)
	url := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s", s.cfg.BitbucketURL, s.projectKey, repoSlug)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		log.Error("A connection error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Debug("response", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))
		return nil, fmt.Errorf("failed to get repo: HTTP %d", resp.StatusCode)
	}

	var summary models.RepoEntity
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		log.Error("An decode error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return nil, err
	}

	repo := &models.RepoEntity{
		Name:        summary.Name,
		Slug:        summary.Slug,
		Description: summary.Description,
		State:       summary.State,
		Public:      summary.Public,
		Forkable:    summary.Forkable,
		Archived:    summary.Archived,
	}

	return repo, nil
}

func (s *RepoService) GetDefaultBranch(repoSlug string) *string {
	log := slog.With("repo", repoSlug)
	url := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/default-branch", s.cfg.BitbucketURL, s.projectKey, repoSlug)

	var result struct {
		ID        string `json:"id"`
		DisplayId string `json:"displayId"`
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		log.Error("A connection error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Debug("response", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Error("A decode error occurred")
		return nil
	}

	return &result.DisplayId
}
