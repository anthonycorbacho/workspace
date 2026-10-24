// Package password provides a library for generating high-entropy random
// password strings via the crypto/rand package.
//
// For example, a 64 characters password with 10 digits and 10 symbols:
//
//	res, err := password.Generate(64, 10, 10)
//	if err != nil {
//		log.Fatal(err)
//	}
//	log.Print(res)
package password
