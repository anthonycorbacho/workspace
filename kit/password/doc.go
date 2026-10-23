// Package password provides a library for generating high-entropy random
// password strings via the crypto/rand package.
//
//	res, err := Generate(64, 10, 10, false, )
//	if err != nil  {
//	  log.Fatal(err)
//	}
//	log.Printf(res)
package password
