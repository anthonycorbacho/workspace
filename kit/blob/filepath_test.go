package blob_test

import (
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
	path := blob.MakeFilePath("438", "test.csv")
	suite.Equal("438/test.csv", path)

	path = blob.MakeFilePath("", "test.csv")
	suite.Equal("global/test.csv", path)
}
