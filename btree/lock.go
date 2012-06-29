//	A linked list of the following structures is stored at BtShared.pLock. Locks are added (or upgraded from READ_LOCK to WRITE_LOCK) when a cursor is opened on the table
//	with root page BtShared.iTable. Locks are removed from this list when a transaction is committed or rolled back, or when a btree handle is closed.

type Lock struct {
	*Btree				//	Btree handle holding this lock
	Table	PageNumber	//	Root page of table
	Lock	byte		//	READ_LOCK or WRITE_LOCK
	Next	*Lock		//	Next in BtShared.pLock list
}

//	Candidate values for Lock.Lock
const(
	READ_LOCK	= 1
	WRITE_LOCK	= iota
)
