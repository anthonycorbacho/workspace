package blob_test

import (
	"strings"
	"testing"

	"github.com/anthonycorbacho/workspace/kit/blob"
	"github.com/stretchr/testify/suite"
)

type FilepathTestSuite struct {
	suite.Suite
}

func TestFilepathTestSuite(t *testing.T) {
	suite.Run(t, new(FilepathTestSuite))
}

func (suite *FilepathTestSuite) TestMakeFilePath() {
	tests := []struct {
		name      string
		companyID string
		filename  string
		want      string
	}{
		{name: "company file", companyID: "438", filename: "test.csv", want: "438/test.csv"},
		{name: "global file", companyID: "", filename: "test.csv", want: "global/test.csv"},
		{name: "nested file", companyID: "438", filename: "reports/2024/test.csv", want: "438/reports/2024/test.csv"},
		{name: "path is cleaned", companyID: "438/", filename: "/reports//test.csv", want: "438/reports/test.csv"},
		{name: "empty filename", companyID: "438", filename: "", want: "438"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.Equal(tt.want, blob.MakeFilePath(tt.companyID, tt.filename))
		})
	}
}

// The company folder scopes the files of a company, a filename must not escape it.
func (suite *FilepathTestSuite) TestMakeFilePathStaysInCompanyFolder() {
	tests := []struct {
		name     string
		filename string
	}{
		{name: "parent directory", filename: "../other/test.csv"},
		{name: "nested parent directory", filename: "reports/../../other/test.csv"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			got := blob.MakeFilePath("438", tt.filename)
			suite.True(strings.HasPrefix(got, "438/"), "file path %q escapes the company folder", got)
		})
	}
}
