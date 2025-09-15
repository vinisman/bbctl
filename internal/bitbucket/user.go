package bitbucket

import (
	"fmt"
	"sync"

	"github.com/vinisman/bbctl/utils"
	openapi "github.com/vinisman/bitbucket-sdk-go/openapi"
)

// getMultipleUsers retrieves multiple users by usernames
func (c *Client) getMultipleUsers(usernames []string) ([]openapi.RestApplicationUser, error) {
	if len(usernames) == 0 {
		return []openapi.RestApplicationUser{}, nil
	}

	c.logger.Info("Getting users", "count", len(usernames))

	// Create a channel to collect results
	resultChan := make(chan *openapi.RestApplicationUser, len(usernames))
	errorChan := make(chan error, len(usernames))
	var wg sync.WaitGroup

	// Process users concurrently
	for _, username := range usernames {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			user, err := c.getSingleUser(u)
			if err != nil {
				errorChan <- fmt.Errorf("failed to get user %s: %w", u, err)
				c.logger.Error("Failed to get user", "username", u, "error", err)
			} else {
				resultChan <- user
				c.logger.Info("Successfully retrieved user", "username", u)
			}
		}(username)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultChan)
		close(errorChan)
	}()

	// Collect results
	var users []openapi.RestApplicationUser
	var errors []error

	// Collect successful results
	for user := range resultChan {
		users = append(users, *user)
	}

	// Collect errors
	for err := range errorChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return users, fmt.Errorf("errors occurred getting users: %v", errors)
	}

	c.logger.Info("Successfully retrieved all users", "count", len(users))
	return users, nil
}

// getSingleUser retrieves a single user by username
func (c *Client) getSingleUser(username string) (*openapi.RestApplicationUser, error) {
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}

	c.logger.Debug("Getting user", "username", username)

	resp, httpResp, err := c.api.SystemMaintenanceAPI.GetUser(c.authCtx, username).Execute()
	if err != nil {
		if httpResp != nil {
			c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
		}
		c.logger.Error("Failed to get user", "username", username, "error", err)
		return nil, err
	}

	c.logger.Debug("Successfully retrieved user", "username", username)
	return resp, nil
}

// getAllUsers retrieves all users from Bitbucket
func (c *Client) getAllUsers() ([]openapi.RestApplicationUser, error) {
	c.logger.Info("Getting all users from Bitbucket")

	resp, httpResp, err := c.api.PermissionManagementAPI.GetUsers1(c.authCtx).Execute()
	if err != nil {
		if httpResp != nil {
			c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
		}
		c.logger.Error("Failed to get all users", "error", err)
		return []openapi.RestApplicationUser{}, err
	}

	// Convert RestDetailedUser to RestApplicationUser
	users := make([]openapi.RestApplicationUser, len(resp.Values))
	for i, detailedUser := range resp.Values {
		users[i] = openapi.RestApplicationUser{
			Name:         detailedUser.Name,
			DisplayName:  detailedUser.DisplayName,
			EmailAddress: detailedUser.EmailAddress,
			Active:       detailedUser.Active,
			Id:           detailedUser.Id,
			Slug:         detailedUser.Slug,
			Type:         detailedUser.Type,
			AvatarUrl:    detailedUser.AvatarUrl,
			Links:        detailedUser.Links,
		}
	}

	c.logger.Info("Successfully retrieved all users", "count", len(users))
	return users, nil
}

// GetUsers retrieves multiple users by usernames (public method)
func (c *Client) GetUsers(usernames []string) ([]openapi.RestApplicationUser, error) {
	return c.getMultipleUsers(usernames)
}

// GetAllUsers retrieves all users from Bitbucket (public method)
func (c *Client) GetAllUsers() ([]openapi.RestApplicationUser, error) {
	return c.getAllUsers()
}

// UserWithPassword represents a user with password for creation
type UserWithPassword struct {
	User     openapi.RestApplicationUser
	Password string
}

// CreateUsers creates multiple users
func (c *Client) CreateUsers(users []openapi.RestApplicationUser, passwords []string) ([]openapi.RestApplicationUser, error) {
	if len(users) == 0 {
		return []openapi.RestApplicationUser{}, nil
	}

	c.logger.Info("Creating users", "count", len(users))

	type result struct {
		index int
		user  *openapi.RestApplicationUser
		err   error
	}

	type job struct {
		index    int
		user     openapi.RestApplicationUser
		password string
	}

	resultsCh := make(chan result, len(users))
	jobs := make(chan job, len(users))

	// Start workers
	maxWorkers := 5 // Use same as projects
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Build the request using the fluent API
				req := c.api.PermissionManagementAPI.CreateUser(c.authCtx).Name(*j.user.Name)

				if j.user.DisplayName != nil {
					req = req.DisplayName(*j.user.DisplayName)
				}
				if j.user.EmailAddress != nil {
					req = req.EmailAddress(*j.user.EmailAddress)
				}

				// Set default values
				req = req.AddToDefaultGroup(true).Notify(false)

				// Set the provided password
				req = req.Password(j.password)

				// Execute the request
				httpResp, err := req.Execute()
				if err != nil {
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					}
					resultsCh <- result{index: j.index, user: &j.user, err: err}
				} else {
					// Return the original user data since the API doesn't return the created user
					resultsCh <- result{index: j.index, user: &j.user}
				}
			}
		}()
	}

	// Send jobs
	for i, user := range users {
		jobs <- job{index: i, user: user, password: passwords[i]}
	}
	close(jobs)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	createdUsers := make([]openapi.RestApplicationUser, len(users))
	var errorsCount int
	for r := range resultsCh {
		if r.err != nil {
			c.logger.Error("Failed to create user", "username", utils.SafeValue(r.user.Name), "error", r.err)
			errorsCount++
			// Keep original user in case of error
			if r.user != nil {
				createdUsers[r.index] = *r.user
			}
		} else {
			c.logger.Info("Created user", "username", utils.SafeValue(r.user.Name))
			createdUsers[r.index] = *r.user
		}
	}

	if errorsCount > 0 {
		return createdUsers, fmt.Errorf("failed to create %d out of %d users", errorsCount, len(users))
	}

	return createdUsers, nil
}

// DeleteUsers deletes multiple users by usernames
func (c *Client) DeleteUsers(usernames []string) ([]openapi.RestApplicationUser, error) {
	if len(usernames) == 0 {
		return []openapi.RestApplicationUser{}, nil
	}

	c.logger.Info("Deleting users", "count", len(usernames))

	type result struct {
		index int
		user  *openapi.RestApplicationUser
		err   error
	}

	type job struct {
		index    int
		username string
	}

	resultsCh := make(chan result, len(usernames))
	jobs := make(chan job, len(usernames))

	// Start workers
	maxWorkers := 5
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Delete user
				_, httpResp, err := c.api.PermissionManagementAPI.DeleteUser(c.authCtx).Name(j.username).Execute()
				if err != nil {
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					}
					resultsCh <- result{index: j.index, user: nil, err: err}
				} else {
					// Create a minimal user object with just the username
					user := &openapi.RestApplicationUser{
						Name: &j.username,
					}
					resultsCh <- result{index: j.index, user: user}
				}
			}
		}()
	}

	// Send jobs
	for i, username := range usernames {
		jobs <- job{index: i, username: username}
	}
	close(jobs)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	deletedUsers := make([]openapi.RestApplicationUser, len(usernames))
	var errorsCount int
	for r := range resultsCh {
		if r.err != nil {
			c.logger.Error("Failed to delete user", "username", usernames[r.index], "error", r.err)
			errorsCount++
		} else {
			c.logger.Info("Deleted user", "username", utils.SafeValue(r.user.Name))
			deletedUsers[r.index] = *r.user
		}
	}

	if errorsCount > 0 {
		return deletedUsers, fmt.Errorf("failed to delete %d out of %d users", errorsCount, len(usernames))
	}

	return deletedUsers, nil
}

// UpdateUsers updates multiple users
func (c *Client) UpdateUsers(users []openapi.RestApplicationUser) ([]openapi.RestApplicationUser, error) {
	if len(users) == 0 {
		return []openapi.RestApplicationUser{}, nil
	}

	c.logger.Info("Updating users", "count", len(users))

	type result struct {
		index int
		user  *openapi.RestApplicationUser
		err   error
	}

	type job struct {
		index int
		user  openapi.RestApplicationUser
	}

	resultsCh := make(chan result, len(users))
	jobs := make(chan job, len(users))

	// Start workers
	maxWorkers := 5
	var wg sync.WaitGroup

	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Get current user data first
				currentUser, err := c.getSingleUser(*j.user.Name)
				if err != nil {
					resultsCh <- result{index: j.index, user: nil, err: err}
					continue
				}

				// Update user data
				updatedUser := *currentUser
				if j.user.DisplayName != nil {
					updatedUser.DisplayName = j.user.DisplayName
				}
				if j.user.EmailAddress != nil {
					updatedUser.EmailAddress = j.user.EmailAddress
				}

				// Update user via API
				_, httpResp, err := c.api.PermissionManagementAPI.UpdateUserDetails(c.authCtx).UserUpdate(openapi.UserUpdate{
					Name:        updatedUser.Name,
					DisplayName: updatedUser.DisplayName,
					Email:       updatedUser.EmailAddress,
				}).Execute()
				if err != nil {
					if httpResp != nil {
						c.logger.Debug("HTTP response", "status", httpResp.StatusCode, "body", httpResp.Body)
					}
					resultsCh <- result{index: j.index, user: currentUser, err: err}
				} else {
					resultsCh <- result{index: j.index, user: &updatedUser}
				}
			}
		}()
	}

	// Send jobs
	for i, user := range users {
		jobs <- job{index: i, user: user}
	}
	close(jobs)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	updatedUsers := make([]openapi.RestApplicationUser, len(users))
	var errorsCount int
	for r := range resultsCh {
		if r.err != nil {
			username := "unknown"
			if r.user != nil {
				username = utils.SafeValue(r.user.Name)
			} else {
				// If user is nil, get username from original users array
				username = utils.SafeValue(users[r.index].Name)
			}
			c.logger.Error("Failed to update user", "username", username, "error", r.err)
			errorsCount++
			// Keep original user in case of error
			if r.user != nil {
				updatedUsers[r.index] = *r.user
			}
		} else {
			c.logger.Info("Updated user", "username", utils.SafeValue(r.user.Name))
			updatedUsers[r.index] = *r.user
		}
	}

	if errorsCount > 0 {
		return updatedUsers, fmt.Errorf("failed to update %d out of %d users", errorsCount, len(users))
	}

	return updatedUsers, nil
}
