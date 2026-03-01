package osv_downloader

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/go-github/v84/github"
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
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func unzipFile(archive_path string, destination string) error {
	fmt.Println("unzipping osv-offline database")
	archive, err := zip.OpenReader(archive_path)
	if err != nil {
		return err
	}
	defer archive.Close()

	for _, file := range archive.File {
		filePath := filepath.Join(destination, file.Name)
		if file.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return err
		}
		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		fileInArchive, err := file.Open()
		if err != nil {
			return err
		}
		if _, err := io.Copy(destFile, fileInArchive); err != nil {
			return err
		}
		defer destFile.Close()
		defer fileInArchive.Close()
	}
	return nil
}
