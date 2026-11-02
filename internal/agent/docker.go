package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	dockerAPIVersion     = "1.41"
	dockerRequestTimeout = 5 * time.Second
)

type dockerStatusError struct {
	path    string
	status  int
	message string
}

func (e *dockerStatusError) Error() string {
	return fmt.Sprintf("docker returned %d for %s: %s", e.status, e.path, e.message)
}

type dockerClient struct {
	http *http.Client
	base string
	host string

	mu      sync.Mutex
	version string
}

type dockerInfo struct {
	Name            string
	ServerVersion   string
	OperatingSystem string
	Architecture    string
	NCPU            int
	MemTotal        int64
}

type dockerContainer struct {
	ID      string `json:"Id"`
	Names   []string
	Image   string
	ImageID string
	State   string
	Status  string
	Labels  map[string]string
	Created int64
	Ports   []dockerPort
}

type dockerPort struct {
	IP          string
	PrivatePort int
	PublicPort  int
	Type        string
}

type dockerImage struct {
	ID          string `json:"Id"`
	RepoDigests []string
}

type dockerEvent struct {
	Type   string
	Action string
	Actor  struct {
		ID string
	}
}

func newDockerClient(dockerHost string) (*dockerClient, error) {
	scheme, address, found := strings.Cut(dockerHost, "://")

	if !found {
		return nil, fmt.Errorf("DOCKER_HOST must start with unix:// or tcp://, got %q", dockerHost)
	}

	client := &dockerClient{host: dockerHost}

	switch scheme {
	case "unix":
		client.base = "http://docker"
		client.http = &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", address)
			},
		}}

	case "tcp":
		client.base = "http://" + address
		client.http = &http.Client{}

	default:
		return nil, fmt.Errorf("DOCKER_HOST must start with unix:// or tcp://, got %q", dockerHost)
	}

	return client, nil
}

func (d *dockerClient) info(ctx context.Context) (dockerInfo, error) {
	var info dockerInfo

	return info, d.get(ctx, "/info", nil, &info)
}

func (d *dockerClient) containers(ctx context.Context) ([]dockerContainer, error) {
	var containers []dockerContainer

	return containers, d.get(ctx, "/containers/json", url.Values{"all": {"1"}}, &containers)
}

func (d *dockerClient) images(ctx context.Context) ([]dockerImage, error) {
	var images []dockerImage

	return images, d.get(ctx, "/images/json", nil, &images)
}

func (d *dockerClient) containerDetails(ctx context.Context, id string) (containerDetails, bool, error) {
	var container struct {
		Config struct{ Image string }
		State  struct {
			StartedAt  string
			FinishedAt string
			ExitCode   int
		}
	}

	err := d.get(ctx, "/containers/"+id+"/json", nil, &container)

	var statusErr *dockerStatusError

	if errors.As(err, &statusErr) && statusErr.status == http.StatusNotFound {
		return containerDetails{}, false, nil
	}

	if err != nil {
		return containerDetails{}, false, err
	}

	return containerDetails{
		image:      container.Config.Image,
		startedAt:  unixSeconds(container.State.StartedAt),
		finishedAt: unixSeconds(container.State.FinishedAt),
		exitCode:   container.State.ExitCode,
	}, true, nil
}

func (d *dockerClient) imageID(ctx context.Context, ref string) (string, error) {
	var image dockerImage

	err := d.get(ctx, "/images/"+ref+"/json", nil, &image)

	var statusErr *dockerStatusError

	if errors.As(err, &statusErr) && statusErr.status == http.StatusNotFound {
		return "", nil
	}

	return image.ID, err
}

func (d *dockerClient) watchEvents(ctx context.Context, onConnect func(), onEvent func(dockerEvent)) error {
	version, err := d.apiVersion(ctx)

	if err != nil {
		return err
	}

	filters, _ := json.Marshal(map[string][]string{"type": {"container", "image"}})
	query := url.Values{"filters": {string(filters)}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+"/v"+version+"/events?"+query.Encode(), nil)

	if err != nil {
		return err
	}

	resp, err := d.http.Do(req)

	if err != nil {
		return d.describe(err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker events returned %s", resp.Status)
	}

	onConnect()

	decoder := json.NewDecoder(resp.Body)

	for {
		var event dockerEvent

		if err := decoder.Decode(&event); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return fmt.Errorf("docker event stream ended: %w", err)
		}

		onEvent(event)
	}
}

func (d *dockerClient) apiVersion(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.version != "" {
		return d.version, nil
	}

	var versions struct {
		MinAPIVersion string
	}

	if err := d.request(ctx, "/version", nil, &versions); err != nil {
		return "", err
	}

	d.version = dockerAPIVersion

	if newerAPIVersion(versions.MinAPIVersion, d.version) {
		d.version = versions.MinAPIVersion
	}

	return d.version, nil
}

func (d *dockerClient) get(ctx context.Context, path string, query url.Values, out any) error {
	version, err := d.apiVersion(ctx)

	if err != nil {
		return err
	}

	return d.request(ctx, "/v"+version+path, query, out)
}

func (d *dockerClient) request(ctx context.Context, path string, query url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, dockerRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+path, nil)

	if err != nil {
		return err
	}

	req.URL.RawQuery = query.Encode()

	resp, err := d.http.Do(req)

	if err != nil {
		return d.describe(err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body struct{ Message string }
		_ = json.NewDecoder(resp.Body).Decode(&body)

		return &dockerStatusError{path: path, status: resp.StatusCode, message: body.Message}
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (d *dockerClient) describe(err error) error {
	var urlErr *url.Error

	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("the Docker daemon at %s didn't answer within %s", d.host, dockerRequestTimeout)
	}

	hint := ""

	switch {
	case errors.Is(err, syscall.ENOENT):
		hint = "; is the Docker socket mounted into the agent container?"
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		hint = "; the agent needs access to the Docker socket, run it as root or in the docker group"
	case errors.Is(err, syscall.ECONNREFUSED):
		hint = "; nothing is listening there, check DOCKER_HOST"
	}

	return fmt.Errorf("cannot read Docker at %s: %w%s", d.host, err, hint)
}

func unixSeconds(value string) int64 {
	t, err := time.Parse(time.RFC3339Nano, value)

	if err != nil || t.Year() < 1970 {
		return 0
	}

	return t.Unix()
}

func newerAPIVersion(a, b string) bool {
	aMajor, aMinor := splitAPIVersion(a)
	bMajor, bMinor := splitAPIVersion(b)

	return aMajor > bMajor || (aMajor == bMajor && aMinor > bMinor)
}

func splitAPIVersion(version string) (int, int) {
	major, minor, _ := strings.Cut(version, ".")
	majorNumber, _ := strconv.Atoi(major)
	minorNumber, _ := strconv.Atoi(minor)

	return majorNumber, minorNumber
}
