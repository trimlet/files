package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>

// trashItem moves the file or folder at path to the Trash, as Finder does
// (it can be put back from there). It returns NULL and sets *where to the path in the Trash
// (to free), or returns the error (to free).
static char *trashItem(const char *path, char **where) {
	@autoreleasepool {
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		NSURL *res = nil;
		NSError *err = nil;
		if (![[NSFileManager defaultManager] trashItemAtURL:url resultingItemURL:&res error:&err]) {
			return strdup([[err localizedDescription] UTF8String]);
		}
		*where = res ? strdup([[res path] UTF8String]) : NULL;
		return NULL;
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// moveToTrash moves path to the Trash and says where it is now, for undo.
func moveToTrash(path string) (string, error) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var where *C.char
	if e := C.trashItem(p, &where); e != nil {
		defer C.free(unsafe.Pointer(e))
		return "", errors.New(C.GoString(e))
	}
	if where == nil {
		return "", nil
	}
	defer C.free(unsafe.Pointer(where))
	return C.GoString(where), nil
}
