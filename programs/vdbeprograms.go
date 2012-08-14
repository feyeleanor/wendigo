//	This VDBE program seeks a btree cursor to the identified db/table/row entry. The reason for using a vdbe program instead of writing code to use the b-tree layer directly is that the vdbe program will take advantage of the various transaction, locking and error handling infrastructure built into the vdbe.
// After seeking the cursor, the vdbe executes an OP_ResultRow. Code external to the Vdbe then "borrows" the b-tree cursor and uses it to implement the blob_read(), blob_write() and blob_bytes() functions.
//	The sqlite3_blob::Close() function finalizes the vdbe program, which closes the b-tree cursor and (possibly) commits the transaction.
const OPEN_BLOB []VdbeOpList = {
	{OP_Transaction, 0, 0, 0},				//	0: Start a transaction
	{OP_VerifyCookie, 0, 0, 0},				//	1: Check the schema cookie
	{OP_TableLock, 0, 0, 0},				//	2: Acquire a read or write lock

	//	One of the following two instructions is replaced by an OP_Noop.
	{OP_OpenRead, 0, 0, 0},					//	3: Open cursor 0 for reading
	{OP_OpenWrite, 0, 0, 0},				//	4: Open cursor 0 for read/write

	{OP_Variable, 1, 1, 1},					//	5: Push the rowid to the stack
	{OP_NotExists, 0, 10, 1},				//	6: Seek the cursor
	{OP_Column, 0, 0, 1},					//	7
	{OP_ResultRow, 1, 0, 0},				//	8
	{OP_Goto, 0, 5, 0},						//	9
	{OP_Close, 0, 0, 0},					//	10
	{OP_Halt, 0, 0, 0},						//	11
}


//	Remove trigger record from database
const DROP_TRIGGER []VdbeOpList = {
	{ OP_Rewind,     0, ADDR(9),  0},
	{ OP_String8,    0, 1,        0},		//	1
	{ OP_Column,     0, 1,        2},
	{ OP_Ne,         2, ADDR(8),  1},
	{ OP_String8,    0, 1,        0},		//	4: "trigger"
	{ OP_Column,     0, 0,        2},
	{ OP_Ne,         2, ADDR(8),  1},
	{ OP_Delete,     0, 0,        0},
	{ OP_Next,       0, ADDR(1),  0},		//	8
}


//	Write the specified cookie value
const SET_COOKIE []VdbeOpList = {
	{ OP_Transaction,    0,  1,  0},		//	0
	{ OP_Integer,        0,  1,  0},		//	1
	{ OP_SetCookie,      0,  0,  1},		//	2
}


//	Read the specified cookie value
const READ_COOKIE []VdbeOpList = {
	{ OP_Transaction,     0,  0,  0},		//	0
	{ OP_ReadCookie,      0,  1,  0},		//	1
	{ OP_ResultRow,       1,  1,  0}
}


const INTEGRITY_CHECK_INDEX_ERROR []VdbeOpList = {
	{ OP_AddImm,      1, -1,  0},
	{ OP_String8,     0,  3,  0},		//	1
	{ OP_Rowid,       1,  4,  0},
	{ OP_String8,     0,  5,  0},		//	3
	{ OP_String8,     0,  6,  0},		//	4
	{ OP_Concat,      4,  3,  3},
	{ OP_Concat,      5,  3,  3},
	{ OP_Concat,      6,  3,  3},
	{ OP_ResultRow,   3,  1,  0},
	{ OP_IfPos,       1,  0,  0},		//	9
	{ OP_Halt,        0,  0,  0},
}


const INTEGRITY_CHECK_COUNT_INDICES []VdbeOpList = {
	{ OP_Integer,      0,  3,  0},
	{ OP_Rewind,       0,  0,  0},		//	1
	{ OP_AddImm,       3,  1,  0},
	{ OP_Next,         0,  0,  0},		//	3
	{ OP_Eq,           2,  0,  3},		//	4
	{ OP_AddImm,       1, -1,  0},
	{ OP_String8,      0,  2,  0},		//	6
	{ OP_String8,      0,  3,  0},		//	7
	{ OP_Concat,       3,  2,  2},
	{ OP_ResultRow,    2,  1,  0},
}


//	Code that appears at the end of an integrity check. If no error messages have been generated, output OK. Otherwise output the error message.
const COMPLETED_INTEGRITY_CHECK []VdbeOpList = {
	{ OP_AddImm,      1, 0,    0},		//	0
	{ OP_IfNeg,       1, 0,    0},		//	1
	{ OP_String8,     0, 3,    0},		//	2
	{ OP_ResultRow,   3, 1,    0},
}


//	When setting the auto_vacuum mode to either "full" or "incremental", write the value of meta[6] in the database file. Before writing to meta[6], check that meta[3] indicates that this really is an auto-vacuum capable database.
const AUTO_VACUUM_SET_META_6 []VdbeOpList = {
	{ OP_Transaction,	0,			1,					0						},		//	0
	{ OP_ReadCookie,	0,			1,					BTREE_LARGEST_ROOT_PAGE	},
	{ OP_If,			1,			0,					0						},		//	2
	{ OP_Halt,			SQLITE_OK,	OE_Abort,			0						},		//	3
	{ OP_Integer,		0,			1,					0						},		//	4
	{ OP_SetCookie,		0,			BTREE_INCR_VACUUM,	1						},		//	5
}
