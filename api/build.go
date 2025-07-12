package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/vinisman/bbctl/utils"
)

type RequiredBuildsService struct {
	cfg        utils.Config
	projectKey string
	client     *http.Client
}

func NewRequiredBuildsService(cfg utils.Config, projectKey string) *RequiredBuildsService {
	return &RequiredBuildsService{cfg: cfg, projectKey: projectKey, client: http.DefaultClient}
}

type condition struct {
	ID              int                    `json:"id"`
	BuildParentKeys []string               `json:"buildParentKeys"`
	RefMatcher      map[string]interface{} `json:"refMatcher"`
}

type conditionsResponse struct {
	Values []condition `json:"values"`
}

// Create or update required build by buildKey
func (s *RequiredBuildsService) CreateOrUpdate(repoSlug string, branch string, buildKeys []string) error {
	log := slog.With("repo", repoSlug)
	base := fmt.Sprintf("%s/rest/required-builds/latest/projects/%s/repos/%s",
		s.cfg.BitbucketURL, s.projectKey, repoSlug)

	// Get conditions list
	resp, err := s.client.Get(base + "/conditions")
	if err != nil {
		log.Error("A connection error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Error("failed to get conditions")
		log.Debug("details", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))

		return fmt.Errorf("failed to fetch conditions: HTTP %d", resp.StatusCode)
	}

	var conds conditionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&conds); err != nil {
		log.Error("A decode error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return err
	}

	// If found condition by buildKey
	for _, c := range conds.Values {

		common := utils.IntersectMap(c.BuildParentKeys, buildKeys)
		if common != nil {
			return s.UpdateCondition(c.ID, branch, common, repoSlug)
		}
		// for _, key := range c.BuildParentKeys {
		// 	if key == buildKey {
		// 		// Update
		// 		return s.UpdateCondition(c.ID, branch, buildKeys, repoSlug)
		// 	}
		// }
	}

	// If not found create
	return s.CreateCondition(branch, buildKeys, repoSlug)
}

// Create confition
func (s *RequiredBuildsService) CreateCondition(branch string, buildKeys []string, repoSlug string) error {
	base := fmt.Sprintf("%s/rest/required-builds/latest/projects/%s/repos/%s",
		s.cfg.BitbucketURL, s.projectKey, repoSlug)
	url := base + "/condition"
	payload := map[string]interface{}{
		"buildParentKeys": buildKeys,
		"refMatcher": map[string]interface{}{
			"displayId": branch,
			"id":        fmt.Sprintf("refs/heads/%s", branch),
			"type":      map[string]string{"name": "BRANCH", "id": "BRANCH"},
		},
	}
	return s.sendRequest("POST", url, payload, repoSlug, "created")
}

// Update condition
func (s *RequiredBuildsService) UpdateCondition(id int, branch string, buildKeys []string, repoSlug string) error {
	base := fmt.Sprintf("%s/rest/required-builds/latest/projects/%s/repos/%s",
		s.cfg.BitbucketURL, s.projectKey, repoSlug)
	url := fmt.Sprintf("%s/condition/%d", base, id)
	payload := map[string]interface{}{
		"id":              id,
		"buildParentKeys": buildKeys,
		"refMatcher": map[string]interface{}{
			"displayId": branch,
			"id":        fmt.Sprintf("refs/heads/%s", branch),
			"type":      map[string]string{"name": "BRANCH", "id": "BRANCH"},
		},
	}
	return s.sendRequest("PUT", url, payload, repoSlug, "updated")
}

func (s *RequiredBuildsService) sendRequest(method string, url string, payload map[string]interface{}, repoSlug, action string) error {
	log := slog.With("repo", repoSlug)
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest(method, url, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		log.Error("A connection error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Info("created", slog.String("repo", repoSlug))
		return nil
	}
	return fmt.Errorf("%s: failed %s required-build, HTTP %d", repoSlug, action, resp.StatusCode)
}

// List all conditions
func (s *RequiredBuildsService) List(repoSlug string) ([]condition, error) {
	log := slog.With("repo", repoSlug)
	url := fmt.Sprintf("%s/rest/required-builds/latest/projects/%s/repos/%s/conditions",
		s.cfg.BitbucketURL, s.projectKey, repoSlug)

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

	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Error("failed to list")
		log.Debug("details", slog.Any("httpCode", resp.StatusCode), slog.String("response", string(bodyBytes)))
		return nil, fmt.Errorf("list conditions failed: HTTP %d", resp.StatusCode)
	}

	var respBody conditionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		log.Error("A decode error occurred")
		log.Debug("An error occurred", slog.String("error", err.Error()))
		return nil, err
	}
	return respBody.Values, nil
}

// Delete condition by buildKey
func (s *RequiredBuildsService) Delete(repoSlug, buildKey string) error {
	log := slog.With("repo", repoSlug)
	base := fmt.Sprintf("%s/rest/required-builds/latest/projects/%s/repos/%s",
		s.cfg.BitbucketURL, s.projectKey, repoSlug)

	conds, err := s.List(repoSlug)
	if err != nil {
		log.Error("Getting repositories list")
		slog.Debug("An error occurred", slog.String("error", err.Error()))
		return err
	}

	for _, c := range conds {
		for _, k := range c.BuildParentKeys {
			if k == buildKey {
				url := fmt.Sprintf("%s/condition/%d", base, c.ID)
				req, _ := http.NewRequest("DELETE", url, nil)
				req.Header.Set("Authorization", "Bearer "+s.cfg.BitbucketToken)
				resp, err := s.client.Do(req)
				if err != nil {
					log.Error("A connection error occurred")
					log.Debug("An error occurred", slog.String("error", err.Error()))
					return err
				}
				defer resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					log.Info("deleted")
					return nil
				}
				return fmt.Errorf("failed delete condition: HTTP %d", resp.StatusCode)
			}
		}
	}

	log.Info("no required-build with key")
	return nil
}
