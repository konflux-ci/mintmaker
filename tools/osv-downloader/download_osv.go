package osv_downloader

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v88/github"
)

// Download and extract the upstream OSV database
func DownloadOsvDb(path string) error {
	fmt.Println("getting osv-offline.zip URL from Github")
	client := github.NewClient(nil)
	release, _, err := client.Repositories.GetLatestRelease(context.Background(), "renovatebot", "osv-offline")
	if err != nil {
		return fmt.Errorf("error when accessing latest osv-offline release: %s", err)
	}

	var zipped_db *github.ReleaseAsset
	for _, asset := range release.Assets {
		if *asset.Name == "osv-offline.zip" {
			zipped_db = asset
			break
		}
	}
	if zipped_db == nil {
		return fmt.Errorf("osv-offline.zip asset couldn't be found in the latest release")
	}
	archive_path := filepath.Join(path, "osv-offline.zip")
	err = downloadFile(*zipped_db.BrowserDownloadURL, archive_path)
	if err != nil {
		return fmt.Errorf("error when downloading the osv-offline file: %s", err)
	}
	err = unzipFile(archive_path, path)
	if err != nil {
		return fmt.Errorf("error when unzipping the osv-offline file: %s", err)
	}
	return nil
}

func downloadFile(url string, filepath string) error {
	fmt.Println("downloading osv-offline database")
	resp, err := http.Get(url) //nolint:gosec // URL comes from the official renovatebot GitHub release
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	out, err := os.Create(filepath) //nolint:gosec // output path is constructed by the caller
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

func unzipFile(archive_path string, destination string) error {
	fmt.Println("unzipping osv-offline database")
	archive, err := zip.OpenReader(archive_path)
	if err != nil {
		return err
	}
	defer func() { _ = archive.Close() }()

	destination = filepath.Clean(destination)
	for _, file := range archive.File {
		filePath := filepath.Join(destination, file.Name) //nolint:gosec // zip entry paths are validated below
		cleanPath := filepath.Clean(filePath)
		// Guard against zip slip: reject entries whose resolved path escapes destination
		// after Join+Clean, e.g. "../../etc/passwd" with destination "/tmp/osv-db" resolves
		// to "/etc/passwd", which is not under "/tmp/osv-db/".
		if !strings.HasPrefix(cleanPath, destination+string(os.PathSeparator)) && cleanPath != destination {
			return fmt.Errorf("invalid file path in archive: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0755); err != nil { //nolint:gosec // need "other" permissions for Tekton prepare-db step
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil { //nolint:gosec // need "other" permissions for Tekton prepare-db step
			return err
		}
		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode()) //nolint:gosec // path validated above
		if err != nil {
			return err
		}
		fileInArchive, err := file.Open()
		if err != nil {
			_ = destFile.Close()
			return err
		}
		if _, err := io.Copy(destFile, fileInArchive); err != nil { //nolint:gosec // archive is from the official renovatebot release
			_ = destFile.Close()
			_ = fileInArchive.Close()
			return err
		}
		if err := destFile.Close(); err != nil {
			_ = fileInArchive.Close()
			return err
		}
		if err := fileInArchive.Close(); err != nil {
			return err
		}
	}
	return nil
}
