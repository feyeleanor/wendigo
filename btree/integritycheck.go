//	This structure is passed around through all the sanity checking routines in order to keep track of some global state information.
//
//	The aRef[] array is allocated so that there is 1 bit for each page in the database. As the integrity-check proceeds, for each page used in
//	the database the corresponding bit is set. This allows integrity-check to detect pages that are used twice and orphaned pages (both of which 
//	indicate corruption).

type IntegrityCheck struct {
	Btree			*BtShared			//	The tree being checked out
	Pager			*Pager				//	The associated pager.  Also accessible by pBt.pPager
	aPgRef			*byte				//	1 bit per page in the db (see above)
	PageNumber							//	Number of pages in the database
	MaxErr			int					//	Stop accumulating errors when this reaches zero
	Errors			int					//	Number of messages written to zErrMsg so far
	errMsg			string				//	Accumulate the error message text here
}