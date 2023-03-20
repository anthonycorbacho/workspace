// Package id is a globally unique id generator suited for web scale.
// Features:
//
//   - Size: 12 bytes (96 bits), smaller than UUID, larger than snowflake
//   - Base32 hex encoded by default (16 bytes storage when transported as printable string)
//   - K-ordered
//   - Embedded time with 1 second precision
//   - Unicity guaranteed for 16,777,216 (24 bits) unique ids per second and per host/process
//
// example:
//
//		// creating a global unique ID.
//		ID := id.New()
//		// Output:
//			9m4e2mr0ui3e8a215n4g
//
//		// Generating prefixed ID in case you want to generate a lot of prefixed id
//		//	eg: prefixed id for databases
//		generator := id.NewGenerator("user")
//		ID := generator.Generate()
// 		//Output:
//			user/9m4e2mr0ui3e8a215n4g
//
package id
