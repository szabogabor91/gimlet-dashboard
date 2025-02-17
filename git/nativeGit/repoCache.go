package nativeGit

import (
	"encoding/json"
	"fmt"
	dashboardConfig "github.com/gimlet-io/gimlet-dashboard/cmd/dashboard/config"
	"github.com/gimlet-io/gimlet-dashboard/git/customScm"
	"github.com/gimlet-io/gimlet-dashboard/git/genericScm"
	"github.com/gimlet-io/gimlet-dashboard/server/streaming"
	"github.com/gimlet-io/go-scm/scm"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const Dir_RWX_RX_R = 0754

var fetchRefSpec = []config.RefSpec{
	"refs/heads/*:refs/heads/*",
}

type RepoCache struct {
	tokenManager customScm.NonImpersonatedTokenManager
	repos        map[string]*git.Repository
	stopCh       chan struct{}
	invalidateCh chan string
	cachePath    string

	// For webhook registration
	goScmHelper *genericScm.GoScmHelper
	config      *dashboardConfig.Config

	clientHub *streaming.ClientHub
}

func NewRepoCache(
	tokenManager customScm.NonImpersonatedTokenManager,
	stopCh chan struct{},
	cachePath string,
	goScmHelper *genericScm.GoScmHelper,
	config *dashboardConfig.Config,
	clientHub *streaming.ClientHub,
) (*RepoCache, error) {
	repoCache := &RepoCache{
		tokenManager: tokenManager,
		repos:        map[string]*git.Repository{},
		stopCh:       stopCh,
		invalidateCh: make(chan string),
		cachePath:    cachePath,
		goScmHelper:  goScmHelper,
		config:       config,
		clientHub:    clientHub,
	}

	const DirRwxRxR = 0754
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		os.MkdirAll(cachePath, DirRwxRxR)
	}
	paths, err := os.ReadDir(cachePath)
	if err != nil {
		return nil, fmt.Errorf("cannot list files: %s", err)
	}

	for _, fileInfo := range paths {
		if !fileInfo.IsDir() {
			continue
		}

		path := filepath.Join(cachePath, fileInfo.Name())
		repo, err := git.PlainOpen(path)
		if err != nil {
			logrus.Warnf("cannot open git repository at %s: %s", path, err)
			continue
		}

		repoCache.repos[strings.ReplaceAll(fileInfo.Name(), "%", "/")] = repo
	}

	return repoCache, nil
}

func (r *RepoCache) Run() {
	for {
		for repoName, _ := range r.repos {
			r.syncGitRepo(repoName)
		}

		select {
		case <-r.stopCh:
			logrus.Info("stopping")
			return
		case repoName := <-r.invalidateCh:
			logrus.Infof("received cache invalidate message for %s", repoName)
			r.syncGitRepo(repoName)
			jsonString, _ := json.Marshal(streaming.StaleRepoDataEvent{
				Repo:           repoName,
				StreamingEvent: streaming.StreamingEvent{Event: streaming.StaleRepoDataEventString},
			})
			r.clientHub.Broadcast <- jsonString
		case <-time.After(30 * time.Second):
		}
	}
}

func (r *RepoCache) syncGitRepo(repoName string) {
	token, user, err := r.tokenManager.Token()
	if err != nil {
		logrus.Errorf("couldn't get scm token: %s", err)
		return
	}

	if _, ok := r.repos[repoName]; !ok {
		logrus.Warnf("could not get repo by name from cache: %s", repoName)
		return // preventing a race condition in cleanup
	}

	err = r.repos[repoName].Fetch(&git.FetchOptions{
		RefSpecs: fetchRefSpec,
		Auth: &http.BasicAuth{
			Username: user,
			Password: token,
		},
		Depth: 100,
		Tags:  git.NoTags,
		Prune: true,
	})
	if err == git.NoErrAlreadyUpToDate {
		return
	}
	if err != nil {
		logrus.Errorf("could not fetch: %s", err)
		r.cleanRepo(repoName)
	}
}

func (r *RepoCache) cleanRepo(repoName string) {
	mutex.Lock()
	delete(r.repos, repoName)
	mutex.Unlock()
}

var mutex = &sync.Mutex{}

func (r *RepoCache) InstanceForRead(repoName string) (*git.Repository, error) {
	mutex.Lock()
	if repo, ok := r.repos[repoName]; ok {
		mutex.Unlock()
		return repo, nil
	}
	repo, err := r.clone(repoName)
	mutex.Unlock()
	go r.registerWebhook(repoName)

	return repo, err
}

func (r *RepoCache) InstanceForWrite(repoName string) (*git.Repository, string, error) {
	tmpPath, err := ioutil.TempDir("", "gitops-")
	if err != nil {
		errors.WithMessage(err, "couldn't get temporary directory")
	}

	mutex.Lock()
	if _, ok := r.repos[repoName]; !ok {
		_, err = r.clone(repoName)
		mutex.Unlock()
		if err != nil {
			errors.WithMessage(err, "couldn't clone")
		}
		go r.registerWebhook(repoName)
	}
	mutex.Unlock()

	repoPath := filepath.Join(r.cachePath, strings.ReplaceAll(repoName, "/", "%"))
	err = copy.Copy(repoPath, tmpPath)
	if err != nil {
		errors.WithMessage(err, "could not make copy of repo")
	}

	copiedRepo, err := git.PlainOpen(tmpPath)
	if err != nil {
		return nil, "", fmt.Errorf("cannot open git repository at %s: %s", tmpPath, err)
	}

	return copiedRepo, tmpPath, nil
}

func (r *RepoCache) CleanupWrittenRepo(path string) error {
	return os.RemoveAll(path)
}

func (r *RepoCache) Invalidate(repoName string) {
	r.invalidateCh <- repoName
}

func (r *RepoCache) clone(repoName string) (*git.Repository, error) {
	repoPath := filepath.Join(r.cachePath, strings.ReplaceAll(repoName, "/", "%"))

	os.RemoveAll(repoPath)
	err := os.MkdirAll(repoPath, Dir_RWX_RX_R)
	if err != nil {
		return nil, errors.WithMessage(err, "couldn't create folder")
	}

	token, user, err := r.tokenManager.Token()
	if err != nil {
		return nil, errors.WithMessage(err, "couldn't get scm token")
	}

	opts := &git.CloneOptions{
		URL: fmt.Sprintf("%s/%s", "https://github.com", repoName),
		Auth: &http.BasicAuth{
			Username: user,
			Password: token,
		},
		Depth: 100,
		Tags:  git.NoTags,
	}

	repo, err := git.PlainClone(repoPath, false, opts)
	if err != nil {
		return nil, errors.WithMessage(err, "couldn't clone")
	}

	err = repo.Fetch(&git.FetchOptions{
		RefSpecs: fetchRefSpec,
		Auth: &http.BasicAuth{
			Username: user,
			Password: token,
		},
		Depth: 100,
		Tags:  git.NoTags,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, errors.WithMessage(err, "couldn't fetch")
	}

	r.repos[repoName] = repo
	return repo, nil
}

func (r *RepoCache) registerWebhook(repoName string) {
	owner, repo := scm.Split(repoName)

	token, _, err := r.tokenManager.Token()
	if err != nil {
		logrus.Errorf("couldn't get scm token: %s", err)
	}

	err = r.goScmHelper.RegisterWebhook(
		r.config.Host,
		token,
		r.config.WebhookSecret,
		owner,
		repo,
	)
	if err != nil {
		logrus.Warnf("could not register webhook for %s: %s", repoName, err)
	}
}
