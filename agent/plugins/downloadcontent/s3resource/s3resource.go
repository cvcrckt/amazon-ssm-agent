// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may not
// use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package s3resource implements the methods to access resources from s3
package s3resource

import (
	"github.com/aws/amazon-ssm-agent/agent/appconfig"
	"github.com/aws/amazon-ssm-agent/agent/fileutil"
	"github.com/aws/amazon-ssm-agent/agent/fileutil/artifact"
	"github.com/aws/amazon-ssm-agent/agent/fileutil/filemanager"
	"github.com/aws/amazon-ssm-agent/agent/jsonutil"
	"github.com/aws/amazon-ssm-agent/agent/log"
	"github.com/aws/amazon-ssm-agent/agent/plugins/downloadcontent/system"
	"github.com/aws/amazon-ssm-agent/agent/s3util"

	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// S3Resource is a struct for the remote resource of type git
type S3Resource struct {
	Info     S3Info
	s3Object s3util.AmazonS3URL
}

// S3Info represents the sourceInfo type sent by runcommand
type S3Info struct {
	Path string `json:"path"`
}

// NewS3Resource is a constructor of type GitResource
func NewS3Resource(log log.T, info string) (s3 *S3Resource, err error) {
	var s3Info S3Info
	var input artifact.DownloadInput

	if s3Info, err = parseSourceInfo(info); err != nil {
		return nil, fmt.Errorf("s3 url parsing failed. %v", err)
	}

	input.SourceURL = s3Info.Path
	return &S3Resource{
		Info: s3Info,
	}, nil
}

// parseSourceInfo unmarshals the information in sourceInfo of type GitInfo and returns it
func parseSourceInfo(sourceInfo string) (s3Info S3Info, err error) {

	if err = jsonutil.Unmarshal(sourceInfo, &s3Info); err != nil {
		return s3Info, fmt.Errorf("Source Info could not be unmarshalled for source type S3. Please check JSON format of SourceInfo - %v", err)
	}

	// Trimming the path in URL to remove any unnecessary spaces
	s3Info.Path = strings.TrimSpace(s3Info.Path)

	return
}

// Download calls download to pull down files or directory from s3
func (s3 *S3Resource) Download(log log.T, filesys filemanager.FileSystem, destPath string) (err error) {
	var fileURL *url.URL
	var unescapedURL string
	var folders []string
	var localFilePath string

	isDirTypeDownloaded := true
	if destPath == "" {
		destPath = appconfig.DownloadRoot
	}
	log.Info("Downloading S3 artifacts from path - ", s3.Info.Path)

	if fileURL, err = url.Parse(s3.Info.Path); err != nil {
		return err
	}
	log.Debug("File URL - ", fileURL.String())

	s3.s3Object = s3util.ParseAmazonS3URL(log, fileURL)
	log.Debug("S3 object - ", s3.s3Object.String())
	// Create an object for the source URL. This can be used to list the objects in the folder
	if folders, err = dep.ListS3Objects(log, s3.s3Object); err != nil {
		return err
	}
	if len(folders) == 0 {
		// In case of a file download, append the filename to folders
		isDirTypeDownloaded = false
		folders = append(folders, s3.s3Object.Key)
	}

	// The URL till the bucket name will be concatenated with the prefix in the loop
	// responsible for download
	for _, files := range folders {
		log.Debug("Name of file - ", files)

		if !isPathType(files) { //Only download in case the URL is a file
			subFolderPath := strings.TrimPrefix(files, s3.s3Object.Key)
			var bucketURL *url.URL
			if bucketURL, err = s3.getS3BucketURLString(log); err != nil {
				return fmt.Errorf("Error while obtaining URL parsing - %v", bucketURL)
			}
			if bucketURL == nil {
				return errors.New("URL obtained is nil")
			}
			log.Debug("S3 bucket URL -", bucketURL.String())
			var input artifact.DownloadInput

			// Obtain the full URL for the file before download

			bucketURL.Path += "/" + files
			input.SourceURL = bucketURL.String()
			log.Debug("SourceURL ", input.SourceURL)
			if unescapedURL, err = url.QueryUnescape(input.SourceURL); err != nil {
				return err
			}
			log.Debug("UnescapedURL ", unescapedURL)
			destinationFile := filepath.Base(unescapedURL)

			//when the s3 key has sub-folders leading to files, those sub-folders need to be created as well
			localFilePath = fileutil.BuildPath(destPath, filepath.Dir(subFolderPath))
			if !isDirTypeDownloaded {
				// if the file path provided exists as a directory or if it is in the format,
				// that would be the localFilePath
				if filesys.Exists(destPath) && filesys.IsDirectory(destPath) || isPathType(destPath) {
					localFilePath = destPath
				} else {
					localFilePath = filepath.Dir(destPath)
					destinationFile = filepath.Base(destPath)
				}
			}
			input.DestinationDirectory = localFilePath
			downloadOutput, err := dep.Download(log, input)
			if err != nil {
				return err
			}

			if err = system.RenameFile(log, filesys, downloadOutput.LocalFilePath, destinationFile); err != nil {
				return fmt.Errorf("Something went wrong when trying to access downloaded content. It is "+
					"possible that the content was not downloaded because the path provided is wrong. %v", err)
			}
		}
	}
	return nil
}

// ValidateLocationInfo ensures that the required parameters of SourceInfo are specified
func (s3 *S3Resource) ValidateLocationInfo() (valid bool, err error) {
	// Path is a mandatory input
	if s3.Info.Path == "" {
		return false, errors.New("S3 source path in SourceInfo must be specified")
	}

	return true, nil
}

// getS3BucketURLString returns the URL up to the bucket name
func (s3 *S3Resource) getS3BucketURLString(log log.T) (Url *url.URL, err error) {

	bucketURL := strings.SplitAfter(s3.Info.Path, s3.s3Object.Bucket)
	log.Debug("Bucket URL is - ", bucketURL[0])
	return url.Parse(bucketURL[0])
}

// isPathType returns if the URL is of path type
func isPathType(folderName string) bool {
	lastCharacter := folderName[len(folderName)-1]
	if os.IsPathSeparator(lastCharacter) {
		return true
	}
	return false
}
