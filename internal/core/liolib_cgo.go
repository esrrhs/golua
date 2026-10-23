//go:build cgo && unix

package core

// Platform OK (unix+cgo): extension ABI for luaL_Stream. Not a PUC 5.4.9
// behavioral surface — see NOTES.md. Unchanged vs golua master.
//
// The half of a file handle that only exists for C: see the note on
// ioStream in liolib.go.
//
// PUC-Lua's file handle is a luaL_Stream -- a FILE* and a close
// function, in that order -- and an extension casts the userdata it is
// handed to one and reads straight out of it; LuaFileSystem's setmode
// does.  The port's handles are Go's, so the block that stands for one
// is filled in as if it were C's, the first time C asks for it: a real
// FILE* on a duplicate of the descriptor, and a close function that is
// only an address.  Both are cleared when the file is closed, which is
// how C sees a closed handle the way it sees one in PUC-Lua.
//
// Without cgo there is no C to hand anything to, and the hooks stay nil.

/*
#include "cdlbridge.h"
*/
import "C"

import "unsafe"

func init() {
	udataBlockFill = func(u *Udata, block unsafe.Pointer) {
		s, ok := u.val.(*ioStream)
		if !ok || s.f == nil {
			return /* not a file handle, or a popen one: no descriptor */
		}
		f := C.cdl_fdopen(C.int(s.f.Fd()))
		if f == nil {
			return
		}
		s.cfp = unsafe.Pointer(f)
		C.cdl_stream_set(block, f)
	}
	ioStreamClosed = func(s *ioStream) {
		if s.cfp == nil {
			return
		}
		/* The block belongs to the userdata, which a '__gc'-driven
		   close has already let go of: 'runFinalizers' frees it once
		   the metamethod is done, so there is nothing to clear. */
		if u := s.uw.Value(); u != nil && u.cblock != nil {
			C.cdl_stream_clear(u.cblock)
		}
		C.cdl_stream_close((*C.FILE)(s.cfp))
		s.cfp = nil
	}
}
