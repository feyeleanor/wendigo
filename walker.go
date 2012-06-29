//	This file contains routines used for walking the parser tree for an SQL statement.

//	Context pointer passed down through the tree-walk.
type Walker struct {
	ExprCallback		func (*Walker, *Expr) int
	SelectCallback		func (*Walker, *Select) int
	*Parse
	union {
		pNC			*NameContext				//	Naming context
		i			int							//	Integer value
		pSrcList	*SrcList					//	FROM clause
	} u
}

//	Return code from the parse-tree walking primitives and their callbacks.
const(
	WRC_Continue = iota		//	Continue down into children
	WRC_Prune				//	Omit children but continue walking siblings
	WRC_Abort				//	Abandon the tree walk

//	Walk an expression tree. Invoke the callback once for each node of the expression, while decending. (In other words, the callback is invoked before visiting children.)
//	The return value from the callback should be one of the WRC_* constants to specify how to proceed with the walk.
//			WRC_Continue      Continue descending down the tree.
//			WRC_Prune         Do not descend into child nodes. But allow the walk to continue with sibling nodes.
//			WRC_Abort         Do no more callbacks. Unwind the stack and return the top-level walk call.
//	The return value from this routine is WRC_Abort to abandon the tree walk and WRC_Continue to continue.
func (w *Walker) Expr(pExpr *Expr) (rc int) {
	if pExpr != nil && w.ExprCallback != nil {
		if rc = w.ExprCallback(w, pExpr); rc == WRC_Continue {
			switch {
			case w.Expr(pExpr.pLeft) != WRC_Continue:
				return WRC_Abort
			case w.Expr(pExpr.pRight) != WRC_Continue:
				return WRC_Abort
			case pExpr.HasProperty(EP_xIsSelect):
				if w.Select(pExpr.x.Select) {
					return WRC_Abort
				}
			default:
				if List(w, pExpr.pList) {
					return WRC_Abort
				}
			}
		}
	}
	return rc & WRC_Abort
}

//	Call Expr() for every expression in list p or until an abort request is seen.
func (w *Walker) ExprList(p *ExprList) int {
	if p != nil {
		for _, pItem := range p.a {
			if w.Expr(pItem.Expr) != WRC_Continue {
				return WRC_Abort
			}
		}
    }
	return WRC_Continue
}

//	Walk all expressions associated with SELECT statement p. Do not invoke the SELECT callback on p, but do (of course) invoke any expr callbacks and SELECT callbacks that come from subqueries. Return WRC_Abort or WRC_Continue.
func (w *Walker) SelectExpr(p *Select) int {
	switch {
	case w.ExprList(p.pEList) != WRC_Continue:
		return WRC_Abort
	case w.Expr(p.Where) != WRC_Continue:
		return WRC_Abort
	case w.ExprList(p.pGroupBy) != WRC_Continue:
		return WRC_Abort
	case w.Expr(p.pHaving) != WRC_Continue:
		return WRC_Abort
	case w.ExprList(p.pOrderBy) != WRC_Continue:
		return WRC_Abort
	case w.Expr(p.pLimit) != WRC_Continue:
		return WRC_Abort
	case w.Expr(p.pOffset) != WRC_Continue:
		return WRC_Abort
	}
  return WRC_Continue;
}

//	Walk the parse trees associated with all subqueries in the FROM clause of SELECT statement p. Do not invoke the select callback on p, but do invoke it on each FROM clause subquery and on any subqueries further down in the tree. Return WRC_Abort or WRC_Continue
func (w *Walker) SelectFrom(p *Select) int {
	for _, item := range p.pSrc {
		if w.Select(item.Select) {
			return WRC_Abort
		}
	}
	return WRC_Continue
} 

//	Call Expr() for every expression in Select statement p. Invoke Select() for subqueries in the FROM clause and on the compound select chain, p.pPrior.
//	Return WRC_Continue under normal conditions. Return WRC_Abort if there is an abort request.
//	If the Walker does not have an xSelectCallback() then this routine is a no-op returning WRC_Continue.
func (w *Walker) Select(p *Select) (rc int) {
	if p == nil || w.SelectCallback == nil {
		return WRC_Continue
	}
	for rc = WRC_Continue; p != nil; p = p.pPrior {
		switch rc = w.SelectCallback(w, p); {
    	case rc != WRC_Continue:
			break
    	case w.SelectExpr(p) != WRC_Continue:
			return WRC_Abort
    	case w.SelectFrom(p) != WRC_Continue:
			return WRC_Abort
		}
	}
	return rc & WRC_Abort
}