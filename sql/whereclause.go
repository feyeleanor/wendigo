package sql

//	An instance of the following structure holds all information about a WHERE clause. Mostly this is a container for one or more WhereTerms.
//	Explanation of OuterConjunction:  For a WHERE clause of the form
//			a AND ((b AND c) OR (d AND e)) AND f
//	There are separate WhereClause objects for the whole clause and for the subclauses "(b AND c)" and "(d AND e)". The OuterConjunction field of the subclauses points to the WhereClause object for the whole clause.
struct WhereClause {
	*Parse
	*WhereMaskSet						//	Mapping of table cursor numbers to bitmasks
	Bitmask								//	Bitmask identifying virtual table cursors
	Terms				[]WhereTerm
	Operator			byte
	OuterConjunction	*WhereClause
	Flags				uint16			//	Might include WHERE_AND_ONLY
}

//	Initialize a preallocated WhereClause structure.
func (pWC *WhereClause) Init(pParse *Parse, pMaskSet *WhereMaskSet, Flags uint16) {
	pWC.Parse = pParse
	pWC.WhereMaskSet = pMaskSet
	pWC.OuterConjunction = nil
	pWC.Bitmask = 0
	pWC.Flags = Flags
}
	
//	Deallocate a WhereClause structure. The WhereClause structure itself is not freed. This routine is the inverse of Init().
func (pWC *WhereClause) Clear() {
	db := pWC.Parse.db
	for i, a := len(pWC.Terms) - 1, pWC.Terms; i >= 0; i--, a++ {
		if a.wtFlags & TERM_DYNAMIC {
			db.ExprDelete(a.Expr)
		}
		switch {
		case a.wtFlags & TERM_ORINFO:
			whereOrInfoDelete(db, a.u.pOrInfo)
		case a.wtFlags & TERM_ANDINFO:
			whereAndInfoDelete(db, a.u.pAndInfo)
		}
	}
}

//	Add a single new WhereTerm entry to the WhereClause object pWC. The new WhereTerm object is constructed from Expr p and with wtFlags. The index in pWC.Terms[] of the new WhereTerm is returned on success.
//	0 is returned if the new WhereTerm could not be added due to a memory allocation error. The memory allocation failure will be recorded in the db.mallocFailed flag so that higher-level functions can detect it.
//	This routine will increase the size of the pWC.Terms[] array as necessary.
//	If the wtFlags argument includes TERM_DYNAMIC, then responsibility for freeing the expression p is assumed by the WhereClause object pWC.
//	This is true even if this routine fails to allocate a new WhereTerm.
//	WARNING: This routine might reallocate the space used to store WhereTerms. All pointers to WhereTerms should be invalidated after calling this routine. Such pointers may be reinitialized by referencing the pWC.Terms[] array.
func (pWC *WhereClause) Insert(p *Expr, wtFlags byte) (idx int) {
	pWC.Terms = append(pWC.Terms, &WhereTerm{ pExpr: p, wtFlags: wtFlags, pWC: pWC, iParent: -1 })
	return idx
}

//	This routine identifies subexpressions in the WHERE clause where each subexpression is separated by the AND operator or some other operator specified in the op parameter. The WhereClause structure is filled with pointers to subexpressions. For example:
//			WHERE  a=='hello' AND coalesce(b,11)<10 AND (c+12!=d OR c==22)
//			   \________/     \_______________/     \________________/
//				slot[0]            slot[1]               slot[2]
//	The original WHERE clause in pExpr is unaltered. All this routine does is make slot[] entries point to substructure within pExpr.
//	In the previous sentence and in the diagram, "slot[]" refers to the WhereClause.a[] array. The slot[] array grows as needed to contain all terms of the WHERE clause.
func (pWC *WhereClause) Split(pExpr *Expr, op int) {
	pWC.Operator = byte(op)
	if pExpr != nil {
		if pExpr.op != op {
			pWC.Insert(pExpr, 0)
		} else {
			pWC.Split(pExpr.pLeft, op)
			pWC.Split(pExpr.pRight, op)
		}
	}
}
