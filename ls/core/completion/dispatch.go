// Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
//
// WSO2 LLC licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except in compliance
// with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package completion

import "github.com/ballerina-nutcracker/ballerina/ls/protocol"

// handler pairs a cursor predicate with a candidate builder, passing the
// matched context from match to build.
type handler struct {
	name  string
	match func(*cursor) (any, bool)
	build func(*cursor, any) []protocol.CompletionItem
}

// handlers is ordered by priority; dispatch returns the first match's result,
// even when it is empty.
var handlers = []handler{
	typeDefinitionHandler,
	enumDeclarationHandler,
	fieldAccessHandler,
	invocationHandler,
	importHandler,
	functionDefinitionHandler,
	moduleHandler,
	blockHandler,
	lexicalHandler,
}

func dispatch(c *cursor) []protocol.CompletionItem {
	for _, h := range handlers {
		if ctx, ok := h.match(c); ok {
			return h.build(c, ctx)
		}
	}
	return emptyItems
}
