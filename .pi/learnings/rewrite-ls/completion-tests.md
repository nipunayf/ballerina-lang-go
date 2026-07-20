# Java LS completion test infrastructure

Keep entries summarized and pointer-dense — `path` + symbol, one line each.
Root: `ballerina-lang/language-server/modules/langserver-core/src/test/`.

## Test class hierarchy

- `AbstractLSTest` (`java/org/ballerinalang/langserver/AbstractLSTest.java:52`): base for all LS tests. Initializes `BallerinaLanguageServer` + `Endpoint`, mocks `LSPackageLoader` with 4 package sources. `loadMockedPackages()` hook enables mock injection.
- `CompletionTest` (`java/.../completion/CompletionTest.java:37`): abstract base. `TestConfig` inner class (position, source, description, items, triggerCharacter). `getConfigsList()` walks `resources/completion/<dir>/config/*.json`. `test()`: read config → open doc → `textDocument/completion` → assert via `CompletionTestUtil.isSubList()`.
- 30+ concrete subclasses, one per resource subdirectory: ActionNodeContext, AnnotationContext, AnnotationDeclaration, ClassDefContext, CommentContext, EnumContext, ExpressionContext, FieldAccessExpressionContext, FunctionBody, FunctionDefinition, ImportDeclaration, LetExpressionContext, ListenerDeclaration, MarkdownDocumentationContext, ModuleConstantContext, ModulePartNodeContext, ModuleVariableContext, ModuleXmlnsDeclaration, NaturalExpressionContext, QueryExpressionContext, RecordTypeDescriptor, ServiceBody, ServiceDeclaration, StatementContext, TemplateExpressionNodeContext, TypeDefinition, TypeDescContext, VariableDeclarationContext, WorkspaceProjectCompletion, XMLTypeDescContext.
- `CompletionPerformanceTest` (`CompletionPerformanceTest.java:38`): overrides `getResponse()` to measure latency vs system-property threshold; disabled by default.
- `BallerinaTomlCompletionTest` (`toml/ballerinatoml/completion/BallerinaTomlCompletionTest.java:37`): separate hierarchy (does NOT extend `CompletionTest`/`AbstractLSTest`); own `init()`/`cleanupLanguageServer()`; fixtures under `resources/toml/ballerina_toml/completion/`.

## Config fixture schema

Each `completion/<group>/config/<name>.json`:
- `position`: `{line, character}` — cursor position
- `source`: path relative to `resources/completion/` to the `.bal` file
- `description`: string (often `""`)
- `items`: array of `CompletionItem` — `label`, `kind`, `detail`, `sortText`, `filterText`, `insertText`, `insertTextFormat`, optional `additionalTextEdits` (`{range, newText}`), `documentation`, `labelDetails`
- `triggerCharacter`: optional (e.g. `">"`)

## Assertion mechanism

- `CompletionTestUtil.isSubList()` (`completion/util/CompletionTestUtil.java:68`): converts both lists to property strings via `getCompletionItemPropertyString()` (insertText + detail + label + sortText + filterText + additionalTextEdits), then `list2.containsAll(list1) && list1.size() == list2.size()` — exact content match, order-agnostic.
- `getCompletionItemPropertyString()` (`CompletionTestUtil.java:30`): normalizes `\r\n` → `\n`.
- On failure, logs mismatched items both directions, then `Assert.fail()`.
- `updateConfig()` exists (commented out in normal runs) — can rewrite config JSONs from actual responses for maintenance.

## Fixture counts & groups

- **1,480 JSON configs** and **1,405 `.bal` sources** across ~31 groups under `resources/completion/`.
- Largest groups: `expression_context` (332), `statement_context` (170), `query_expression` (146), `annotation_ctx` (118), `field_access_expression_context` (106), `typedesc_context` (96), `action_node_context` (67), `template_expression` (65).
- Import declaration: `import_decl` (33 configs) — covers `import <cursor>`, `import b<cursor>`, `import ballerina/<cursor>`, `import ballerina/mod<cursor>`, `import ballerina/mod v<cursor>`, `import ballerina/mod.<cursor>`, `import ballerina/mod as <cursor>`, current-project module suggestions, ballerinax central packages, partial module names, no-module-name edge cases.
- Module part: `module_part_context` (44 configs) — covers empty file, cursor at `f`, cursor after import+function, qualified name reference (`module1:`), after annotation/class/const/enum/function/listener/service/type/var/xmlns declarations (both before and after each), after configurable qualifier.
- All groups: action_node_context, annotation_ctx, annotation_decl, class_def, comment_context, enum_decl_ctx, expression_context, field_access_expression_context, function_body, function_def, import_decl, let_expression_context, listener_decl, markdown_documentation_context, module_const_context, module_part_context, module_var_context, module_xml_namespace_decl, natural_expression, query_expression, record_type_desc, service_body, service_decl, statement_context, template_expression, type_def, typedesc_context, variable-declaration, workspace_project, xml_typedesc_context, performance_completion (1 config).

## Skip lists

- `StatementContextTest`: skips 6 configs (elseif, match, start_action)
- `ExpressionContextTest`: skips 7 configs (object_constructor, conditional_expr, method_call_expression, mapping_constructor)
- `WorkspaceProjectCompletionTest`: skips `config1.json` ("TODO: Remove after providing completions for module imports")

## Mocked packages

- `AbstractLSTest` static initializer loads 4 sources via `LSPackageLoader` mock:
  - `REMOTE_PROJECTS`: `project1/main.bal`, `project2/main.bal` (from `resources/repository_projects/`)
  - `LOCAL_PROJECTS`: `local_project1/main.bal`, `local_project2/main.bal`
  - `DISTRIBUTION_PACKAGES`: `BallerinaDistribution.packageRepository()`, skipping 3 lang libs
  - `CENTRAL_PACKAGES`: `resources/central/centralPackages.json`
- Repository projects have `Ballerina.toml` with `[package] org="test" name="project1" version="0.0.1"`.

## TestNG configuration

- `resources/testng.xml`: single suite `language-server-core-test-suite`, `preserve-order="true"`, excludes `broken` group; includes all completion test classes.
