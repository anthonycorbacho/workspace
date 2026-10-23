package blob

import "path"

// MakeFilePath returns the path of the file in the company folder (or "global" if no company is given).
// The filename is cleaned as an absolute path, so it can't escape the company folder (eg: "../other").
func MakeFilePath(companyID string, filename string) string {
	if len(companyID) == 0 {
		companyID = "global"
	}

	return path.Join(companyID, path.Clean("/"+filename))
}
