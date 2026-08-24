// Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package query

import (
	"strings"

	"github.com/ballerina-nutcracker/ballerina/ls/core/uri"
	"github.com/ballerina-nutcracker/ballerina/ls/core/workspace"
	"github.com/ballerina-nutcracker/ballerina/ls/protocol"
	tree "github.com/ballerina-nutcracker/ballerina/st"
)

type ByteRange struct {
	StartLine uint32
	StartChar uint32
	EndLine   uint32
	EndChar   uint32
}

type DocumentSymbol struct {
	Name       string
	Kind       protocol.SymbolKind
	Range      ByteRange
	Deprecated bool
	Children   []DocumentSymbol
}

type Service struct {
	projects *workspace.ProjectService
}

func New(projects *workspace.ProjectService) *Service {
	return &Service{projects: projects}
}

func (s *Service) DocumentSymbols(u uri.DocumentURI) []DocumentSymbol {
	if s == nil || s.projects == nil {
		return nil
	}
	project, err := s.projects.Project(u)
	if err != nil || project == nil {
		return nil
	}
	documentID, ok := project.DocumentID(u.Path())
	if !ok {
		return nil
	}
	module := project.CurrentPackage().Module(documentID.ModuleID())
	if module == nil {
		return nil
	}
	document := module.Document(documentID)
	if document == nil || document.SyntaxTree() == nil {
		return nil
	}
	modulePart, ok := document.SyntaxTree().RootNode.(*tree.ModulePart)
	if !ok {
		return nil
	}
	members := modulePart.Members()
	symbols := make([]DocumentSymbol, 0, members.Size())
	for i := range members.Size() {
		if symbol, ok := symbolForNode(members.Get(i)); ok {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

func symbolForNode(node tree.Node) (DocumentSymbol, bool) {
	switch node := node.(type) {
	case *tree.FunctionDefinition:
		name := tokenText(node.FunctionName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		kind := protocol.SymbolKindFunction
		resourcePath := node.RelativeResourcePath()
		if resourcePath.Size() > 0 {
			name += ":" + nodeListSource(resourcePath)
		}
		return newSymbol(name, kind, node, isDeprecated(node.Metadata()), nil), true
	case *tree.TypeDefinitionNode:
		name := tokenText(node.TypeName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		kind := protocol.SymbolKindTypeParameter
		var children []DocumentSymbol
		switch descriptor := node.TypeDescriptor().(type) {
		case *tree.RecordTypeDescriptorNode:
			kind = protocol.SymbolKindStruct
			children = symbolsForNodes(descriptor.Fields())
			if rest := descriptor.RecordRestDescriptor(); rest != nil {
				if symbol, ok := symbolForNode(rest); ok {
					children = append(children, symbol)
				}
			}
		case *tree.ObjectTypeDescriptorNode:
			kind = protocol.SymbolKindInterface
			children = methodSymbols(descriptor.Members())
		}
		return newSymbol(name, kind, node, isDeprecated(node.Metadata()), children), true
	case *tree.ServiceDeclarationNode:
		return newSymbol("service "+nodeListSource(node.AbsoluteResourcePath())+" ", protocol.SymbolKindObject, node,
			isDeprecated(node.Metadata()), symbolsForNodes(node.Members())), true
	case *tree.ListenerDeclarationNode:
		name := tokenText(node.VariableName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindObject, node, isDeprecated(node.Metadata()), nil), true
	case *tree.ConstantDeclarationNode:
		name := tokenText(node.VariableName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindConstant, node, isDeprecated(node.Metadata()), nil), true
	case *tree.ModuleVariableDeclarationNode:
		pattern := node.TypedBindingPattern()
		if pattern == nil {
			return DocumentSymbol{}, false
		}
		capture, ok := pattern.BindingPattern().(*tree.CaptureBindingPatternNode)
		if !ok {
			return DocumentSymbol{}, false
		}
		name := tokenText(capture.VariableName())
		if name == "" || name == "_" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindVariable, node, isDeprecated(node.Metadata()), nil), true
	case *tree.AnnotationDeclarationNode:
		name := tokenText(node.AnnotationTag())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindProperty, node, isDeprecated(node.Metadata()), nil), true
	case *tree.ModuleXMLNamespaceDeclarationNode:
		name := tokenText(node.NamespacePrefix())
		if name == "" {
			name = "xmlns " + nodeSource(node.Namespaceuri())
		}
		return newSymbol(name, protocol.SymbolKindNamespace, node, false, nil), true
	case *tree.EnumDeclarationNode:
		name := tokenText(node.Identifier())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindEnum, node, isDeprecated(node.Metadata()), symbolsForNodes(node.EnumMemberList())), true
	case *tree.ClassDefinitionNode:
		name := tokenText(node.ClassName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindClass, node, isDeprecated(node.Metadata()), methodSymbols(node.Members())), true
	case *tree.MethodDeclarationNode:
		name := tokenText(node.MethodName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindMethod, node, isDeprecated(node.Metadata()), nil), true
	case *tree.ObjectFieldNode:
		name := tokenText(node.FieldName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindField, node, isDeprecated(node.Metadata()), nil), true
	case *tree.RecordFieldNode:
		name := tokenText(node.FieldName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindField, node, isDeprecated(node.Metadata()), nil), true
	case *tree.RecordFieldWithDefaultValueNode:
		name := tokenText(node.FieldName())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindField, node, isDeprecated(node.Metadata()), nil), true
	case *tree.RecordRestDescriptorNode:
		return newSymbol("..."+nodeSource(node.TypeName()), protocol.SymbolKindField, node, false, nil), true
	case *tree.EnumMemberNode:
		name := tokenText(node.Identifier())
		if name == "" {
			return DocumentSymbol{}, false
		}
		return newSymbol(name, protocol.SymbolKindEnumMember, node, isDeprecated(node.Metadata()), nil), true
	default:
		return DocumentSymbol{}, false
	}
}

func symbolsForNodes[T tree.Node](nodes tree.NodeList[T]) []DocumentSymbol {
	symbols := make([]DocumentSymbol, 0, nodes.Size())
	for i := range nodes.Size() {
		if symbol, ok := symbolForNode(nodes.Get(i)); ok {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

func methodSymbols[T tree.Node](nodes tree.NodeList[T]) []DocumentSymbol {
	symbols := symbolsForNodes(nodes)
	for i := range symbols {
		if symbols[i].Kind != protocol.SymbolKindFunction {
			continue
		}
		if symbols[i].Name == "init" {
			symbols[i].Kind = protocol.SymbolKindConstructor
			continue
		}
		symbols[i].Kind = protocol.SymbolKindMethod
	}
	return symbols
}

func newSymbol(name string, kind protocol.SymbolKind, node tree.Node, deprecated bool, children []DocumentSymbol) DocumentSymbol {
	return DocumentSymbol{
		Name:       name,
		Kind:       kind,
		Range:      byteRange(node),
		Deprecated: deprecated,
		Children:   children,
	}
}

func byteRange(node tree.Node) ByteRange {
	lineRange := node.LineRange()
	return ByteRange{
		StartLine: uint32(lineRange.StartLine.Line),
		StartChar: uint32(lineRange.StartLine.Column),
		EndLine:   uint32(lineRange.EndLine.Line),
		EndChar:   uint32(lineRange.EndLine.Column),
	}
}

func tokenText(token tree.Token) string {
	if token == nil || token.IsMissing() {
		return ""
	}
	return token.Text()
}

func nodeSource(node tree.Node) string {
	if node == nil || node.IsMissing() {
		return ""
	}
	syntaxTree := node.SyntaxTree()
	if syntaxTree == nil || syntaxTree.TextDocument() == nil {
		return ""
	}
	textRange := node.TextRange()
	text := syntaxTree.TextDocument().String()
	if textRange.StartOffset < 0 || textRange.EndOffset > len(text) || textRange.EndOffset < textRange.StartOffset {
		return ""
	}
	return text[textRange.StartOffset:textRange.EndOffset]
}

func nodeListSource(nodes tree.NodeList[tree.Node]) string {
	var builder strings.Builder
	for i := range nodes.Size() {
		builder.WriteString(nodeSource(nodes.Get(i)))
	}
	return builder.String()
}

func isDeprecated(metadata *tree.MetadataNode) bool {
	if metadata == nil {
		return false
	}
	annotations := metadata.Annotations()
	for i := range annotations.Size() {
		if nodeSource(annotations.Get(i).AnnotReference()) == "deprecated" {
			return true
		}
	}
	return false
}
