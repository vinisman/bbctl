package bitbucket

import (
	"fmt"
	"sync"

	"github.com/vinisman/bbctl/internal/config"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// GetAllGroups retrieves all groups from Bitbucket
func (c *Client) GetAllGroups() ([]openapi.RestDetailedGroup, error) {
	c.logger.Info("Getting all groups from Bitbucket")

	resp, httpResp, err := c.api.PermissionManagementAPI.GetGroups1(c.authCtx).Execute()
	if err != nil {
		if httpResp != nil {
			c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
		}
		c.logger.Error("Failed to get all groups", "error", err)
		return []openapi.RestDetailedGroup{}, err
	}

	groups := resp.Values
	c.logger.Info("Successfully retrieved all groups", "count", len(groups))
	return groups, nil
}

// GetGroups retrieves specific groups by names
func (c *Client) GetGroups(groupNames []string) ([]openapi.RestDetailedGroup, error) {
	if len(groupNames) == 0 {
		return []openapi.RestDetailedGroup{}, nil
	}

	c.logger.Info("Getting groups", "count", len(groupNames))

	// Get all groups first
	allGroups, err := c.GetAllGroups()
	if err != nil {
		return []openapi.RestDetailedGroup{}, err
	}

	// Create a map for quick lookup
	groupMap := make(map[string]openapi.RestDetailedGroup)
	for _, group := range allGroups {
		if group.Name != nil {
			groupMap[*group.Name] = group
		}
	}

	// Filter groups by requested names
	var groups []openapi.RestDetailedGroup
	var notFound []string

	for _, name := range groupNames {
		if group, ok := groupMap[name]; ok {
			groups = append(groups, group)
			c.logger.Info("Successfully retrieved group", "name", name)
		} else {
			notFound = append(notFound, name)
			c.logger.Error("Group not found", "name", name)
		}
	}

	if len(notFound) > 0 {
		return groups, fmt.Errorf("groups not found: %v", notFound)
	}

	c.logger.Info("Successfully retrieved all groups", "count", len(groups))
	return groups, nil
}

// CreateGroups creates multiple groups
func (c *Client) CreateGroups(groups []openapi.RestDetailedGroup) ([]openapi.RestDetailedGroup, error) {
	if len(groups) == 0 {
		return []openapi.RestDetailedGroup{}, nil
	}

	c.logger.Info("Creating groups", "count", len(groups))

	type result struct {
		index int
		group *openapi.RestDetailedGroup
		err   error
	}

	type job struct {
		index int
		group openapi.RestDetailedGroup
	}

	resultsCh := make(chan result, len(groups))
	jobs := make(chan job, len(groups))

	// Start workers
	maxWorkers := config.GlobalMaxWorkers
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Go(func() {
			for j := range jobs {
				// Build the request
				req := c.api.PermissionManagementAPI.CreateGroup(c.authCtx).Name(*j.group.Name)

				// Execute the request
				createdGroup, httpResp, err := req.Execute()
				if err != nil {
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					}
					resultsCh <- result{index: j.index, group: &j.group, err: fmt.Errorf("failed to create group %s: %w", *j.group.Name, err)}
				} else {
					// Return the created group from API
					resultsCh <- result{index: j.index, group: createdGroup}
				}
			}
		})
	}

	// Send jobs
	for i, group := range groups {
		jobs <- job{index: i, group: group}
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()
	close(resultsCh)

	// Collect results in order
	results := make([]*openapi.RestDetailedGroup, len(groups))
	var errs []error

	for res := range resultsCh {
		if res.err != nil {
			errs = append(errs, res.err)
			c.logger.Error("Failed to create group", "error", res.err)
		} else {
			results[res.index] = res.group
			c.logger.Info("Successfully created group", "name", *res.group.Name)
		}
	}

	// Filter out nil results
	var createdGroups []openapi.RestDetailedGroup
	for _, group := range results {
		if group != nil {
			createdGroups = append(createdGroups, *group)
		}
	}

	if len(errs) > 0 {
		return createdGroups, fmt.Errorf("errors occurred creating groups: %v", errs)
	}

	c.logger.Info("Successfully created all groups", "count", len(createdGroups))
	return createdGroups, nil
}

// DeleteGroups deletes multiple groups
func (c *Client) DeleteGroups(groupNames []string) ([]openapi.RestDetailedGroup, error) {
	if len(groupNames) == 0 {
		return []openapi.RestDetailedGroup{}, nil
	}

	c.logger.Info("Deleting groups", "count", len(groupNames))

	type result struct {
		index int
		group *openapi.RestDetailedGroup
		err   error
	}

	type job struct {
		index     int
		groupName string
	}

	resultsCh := make(chan result, len(groupNames))
	jobs := make(chan job, len(groupNames))

	// Start workers
	maxWorkers := config.GlobalMaxWorkers
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Go(func() {
			for j := range jobs {
				// Delete group
				deletedGroup, httpResp, err := c.api.PermissionManagementAPI.DeleteGroup(c.authCtx).Name(j.groupName).Execute()
				if err != nil {
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					}
					resultsCh <- result{index: j.index, group: nil, err: fmt.Errorf("failed to delete group %s: %w", j.groupName, err)}
				} else {
					// Return the deleted group from API
					resultsCh <- result{index: j.index, group: deletedGroup}
				}
			}
		})
	}

	// Send jobs
	for i, groupName := range groupNames {
		jobs <- job{index: i, groupName: groupName}
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()
	close(resultsCh)

	// Collect results in order
	results := make([]*openapi.RestDetailedGroup, len(groupNames))
	var errs []error

	for res := range resultsCh {
		if res.err != nil {
			errs = append(errs, res.err)
			c.logger.Error("Failed to delete group", "error", res.err)
		} else {
			results[res.index] = res.group
			c.logger.Info("Successfully deleted group", "name", *res.group.Name)
		}
	}

	// Filter out nil results
	var deletedGroups []openapi.RestDetailedGroup
	for _, group := range results {
		if group != nil {
			deletedGroups = append(deletedGroups, *group)
		}
	}

	if len(errs) > 0 {
		return deletedGroups, fmt.Errorf("errors occurred deleting groups: %v", errs)
	}

	c.logger.Info("Successfully deleted all groups", "count", len(deletedGroups))
	return deletedGroups, nil
}
