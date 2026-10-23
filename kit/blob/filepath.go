package blob

import "path"

func MakeFilePath(companyID string, filename string) string {
	if len(companyID) == 0 {
		companyID = "global"
	}

	return path.Join(companyID, filename)
}
