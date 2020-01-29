// The mbox package provides support for reading legacy MBOX-family files.
// Support for writing MBOX files remains unimplemented at this time.
//
// According to Wikipedia (https://en.wikipedia.org/wiki/Mbox), at least four
// different kinds of mutually incompatible MBOX formats exist:
//
//	* mboxo (described by RFC-4155)
//	* mboxrd (providing a reversable >From escaping semantics)
//	* mboxcl (as mboxo, but which honors the Content-Length MIME header)
//	* mboxcl2 (as mboxcl, but which does not escape From lines)
//
// This mbox package should be able to handle all four kinds of mailbox files;
// however, due to the vaguarities of the MBOX family, some of the burden for
// proper decoding must rest with your client software.  For example, mbox
// ignores any Content-Length MIME headers, and performs absolutely minimal
// amounts of processing of the headers so as to not discard potentially useful
// information.  It further will not reverse any >From-escaping that might have
// occured when the file was written.
//
// Using mbox involves creating an io.Reader for the MBOX file, then submitting
// that to the CreateMboxStream() function.  Then, for each message in the MBOX
// file, read that message and process as appropriate.
//
// Note: at this time, no means of seeking through the file exists.  Your
// software must process messages in a sequential, batch-oriented manner.
package mbox
