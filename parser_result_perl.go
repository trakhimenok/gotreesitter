package gotreesitter

func normalizePerlCompatibility(root *Node, source []byte, lang *Language) {
	normalizePerlJoinAssignmentLists(root, source, lang)
	normalizePerlPushExpressionLists(root, source, lang)
	normalizePerlReturnExpressionLists(root, lang)
}

func normalizePerlJoinAssignmentLists(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "perl" {
		return
	}
	listSym, ok := lang.SymbolByName("list_expression")
	if !ok {
		return
	}
	listNamed := false
	if idx := int(listSym); idx < len(lang.SymbolMetadata) {
		listNamed = lang.SymbolMetadata[listSym].Named
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "expression_statement" && len(n.children) == 1 {
			assign := n.children[0]
			if rewritten := rewritePerlJoinAssignmentList(n.ownerArena, assign, source, lang, listSym, listNamed); rewritten != nil {
				n.children[0] = rewritten
				rewritten.parent = n
				rewritten.childIndex = 0
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func rewritePerlJoinAssignmentList(arena *nodeArena, assign *Node, source []byte, lang *Language, listSym Symbol, listNamed bool) *Node {
	if assign == nil || assign.Type(lang) != "assignment_expression" || len(assign.children) != 3 {
		return nil
	}
	call := assign.children[2]
	if call == nil || call.Type(lang) != "ambiguous_function_call_expression" || len(call.children) != 2 {
		return nil
	}
	fn := call.children[0]
	args := call.children[1]
	if fn == nil || args == nil || fn.Text(source) != "join" || args.Type(lang) != "list_expression" || len(args.children) < 3 {
		return nil
	}
	firstArg := args.children[0]
	if firstArg == nil {
		return nil
	}

	callFieldIDs := append([]FieldID(nil), call.fieldIDs...)
	if len(callFieldIDs) > 2 {
		callFieldIDs = callFieldIDs[:2]
	}
	rewrittenCall := newParentNodeInArena(arena, call.symbol, call.isNamed, []*Node{fn, firstArg}, callFieldIDs, call.productionID)
	if len(call.fieldSources) > 0 {
		rewrittenCall.fieldSources = append([]uint8(nil), call.fieldSources...)
		if len(rewrittenCall.fieldSources) > 2 {
			rewrittenCall.fieldSources = rewrittenCall.fieldSources[:2]
		}
	}

	assignFieldIDs := append([]FieldID(nil), assign.fieldIDs...)
	rewrittenAssign := newParentNodeInArena(arena, assign.symbol, assign.isNamed, []*Node{assign.children[0], assign.children[1], rewrittenCall}, assignFieldIDs, assign.productionID)
	if len(assign.fieldSources) > 0 {
		rewrittenAssign.fieldSources = append([]uint8(nil), assign.fieldSources...)
	}

	outerChildren := make([]*Node, 0, len(args.children))
	outerChildren = append(outerChildren, rewrittenAssign)
	outerChildren = append(outerChildren, args.children[1:]...)
	return newParentNodeInArena(arena, listSym, listNamed, outerChildren, nil, args.productionID)
}

func normalizePerlPushExpressionLists(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "perl" {
		return
	}
	listSym, ok := lang.SymbolByName("list_expression")
	if !ok {
		return
	}
	listNamed := false
	if idx := int(listSym); idx < len(lang.SymbolMetadata) {
		listNamed = lang.SymbolMetadata[listSym].Named
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "expression_statement" && len(n.children) == 1 {
			list := n.children[0]
			if rewritten := rewritePerlPushExpressionList(n.ownerArena, list, source, lang, listSym, listNamed); rewritten != nil {
				n.children[0] = rewritten
				rewritten.parent = n
				rewritten.childIndex = 0
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func rewritePerlPushExpressionList(arena *nodeArena, list *Node, source []byte, lang *Language, listSym Symbol, listNamed bool) *Node {
	if list == nil || list.Type(lang) != "list_expression" || len(list.children) < 3 {
		return nil
	}
	call := list.children[0]
	if call == nil || call.Type(lang) != "ambiguous_function_call_expression" || len(call.children) != 2 {
		return nil
	}
	fn := call.children[0]
	firstArg := call.children[1]
	if fn == nil || firstArg == nil || fn.Text(source) != "push" {
		return nil
	}
	argChildren := make([]*Node, 0, len(list.children))
	argChildren = append(argChildren, firstArg)
	argChildren = append(argChildren, list.children[1:]...)
	rewrittenArgs := newParentNodeInArena(arena, listSym, listNamed, argChildren, nil, list.productionID)

	callFieldIDs := append([]FieldID(nil), call.fieldIDs...)
	if len(callFieldIDs) > 2 {
		callFieldIDs = callFieldIDs[:2]
	}
	rewrittenCall := newParentNodeInArena(arena, call.symbol, call.isNamed, []*Node{fn, rewrittenArgs}, callFieldIDs, call.productionID)
	if len(call.fieldSources) > 0 {
		rewrittenCall.fieldSources = append([]uint8(nil), call.fieldSources...)
		if len(rewrittenCall.fieldSources) > 2 {
			rewrittenCall.fieldSources = rewrittenCall.fieldSources[:2]
		}
	}
	return rewrittenCall
}

func normalizePerlReturnExpressionLists(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "perl" {
		return
	}
	listSym, ok := lang.SymbolByName("list_expression")
	if !ok {
		return
	}
	listNamed := false
	if idx := int(listSym); idx < len(lang.SymbolMetadata) {
		listNamed = lang.SymbolMetadata[listSym].Named
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "expression_statement" && len(n.children) == 1 {
			ret := n.children[0]
			if rewritten := rewritePerlReturnExpressionList(n.ownerArena, ret, lang, listSym, listNamed); rewritten != nil {
				n.children[0] = rewritten
				rewritten.parent = n
				rewritten.childIndex = 0
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func rewritePerlReturnExpressionList(arena *nodeArena, ret *Node, lang *Language, listSym Symbol, listNamed bool) *Node {
	if ret == nil || ret.Type(lang) != "return_expression" || len(ret.children) != 2 {
		return nil
	}
	list := ret.children[1]
	if list == nil || list.Type(lang) != "list_expression" || len(list.children) < 3 {
		return nil
	}
	firstItem := list.children[0]
	if firstItem == nil {
		return nil
	}

	retFieldIDs := append([]FieldID(nil), ret.fieldIDs...)
	if len(retFieldIDs) > 2 {
		retFieldIDs = retFieldIDs[:2]
	}
	rewrittenReturn := newParentNodeInArena(arena, ret.symbol, ret.isNamed, []*Node{ret.children[0], firstItem}, retFieldIDs, ret.productionID)
	if len(ret.fieldSources) > 0 {
		rewrittenReturn.fieldSources = append([]uint8(nil), ret.fieldSources...)
		if len(rewrittenReturn.fieldSources) > 2 {
			rewrittenReturn.fieldSources = rewrittenReturn.fieldSources[:2]
		}
	}

	outerChildren := make([]*Node, 0, len(list.children))
	outerChildren = append(outerChildren, rewrittenReturn)
	outerChildren = append(outerChildren, list.children[1:]...)
	return newParentNodeInArena(arena, listSym, listNamed, outerChildren, nil, list.productionID)
}
