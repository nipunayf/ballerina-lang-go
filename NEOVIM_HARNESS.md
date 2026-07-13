Here's a minimal specification focused purely on goals and Neovim-endorsed testing practices, ready to hand to an agent.

## Specification: Neovim LSP Test Harness

### Goal
Set up a headless Neovim instance that attaches its built-in LSP client to a target language server, so an external or embedded agent can send LSP requests, capture structured responses, and assert against expected behavior in a repeatable loop. [neovim](https://neovim.io/doc/user/dev_test/)

### Core Principles (Neovim-endorsed)
- **Headless execution**: All tests run via `nvim --headless` or `nvim -l script.lua` — no UI, no interactive prompts. [mrcjkb](https://mrcjkb.dev/posts/2023-06-06-luarocks-test.html)
- **RPC-driven, not UI-driven**: Neovim's own functional test suite favors driving via RPC/API calls rather than simulating keystrokes, since this is faster and more deterministic. [neovim](https://neovim.io/doc/user/dev_test/)
- **Isolated per-test instance**: Each test/spec should start a fresh Nvim process (or fresh state) and discard it afterward to avoid cross-test contamination. [neovim](https://neovim.io/doc/user/dev_test/)
- **Minimal init file**: Use a dedicated minimal `init.lua`/`minimal.vim` that loads only the runtime path entries needed (test framework + LSP config), not the user's full config. [zignar](https://zignar.net/2022/10/26/testing-neovim-lsp-plugins/)
- **Real client, not a mock**: Rely on Neovim's built-in `vim.lsp` client rather than reimplementing JSON-RPC, so tests exercise genuine capability negotiation and document sync. [neovim](https://neovim.io/doc/user/lsp/)

### Required Setup Components
- **Test runner**: Use `busted` (via `nlua` as the Lua interpreter) or `plenary.nvim`'s `PlenaryBustedDirectory` — both are the community-standard, Neovim-compatible frameworks. [zignar](https://zignar.net/2022/10/26/testing-neovim-lsp-plugins/)
- **Minimal init file**: A `tests/minimal_init.lua` that sets `runtimepath`, loads the test framework plugin, and configures the target LSP server via `vim.lsp.start()` or `nvim-lspconfig`. [github](https://github.com/lewis6991/nvim-test)
- **Spec files**: Name test files `*_spec.lua`; organize by concept/feature (e.g. `hover_spec.lua`, `completion_spec.lua`), following Neovim's own convention of grouping functional tests by semantic area. [neovim](https://neovim.io/doc/user/dev_test/)
- **Assertions**: Use `assert.are.same()` / `luassert` to compare LSP response tables against expected structures. [zignar](https://zignar.net/2022/10/26/testing-neovim-lsp-plugins/)

### Required Agent-Facing Interface
- **Request dispatch**: Expose a function to send arbitrary LSP methods (`textDocument/hover`, `textDocument/definition`, `textDocument/completion`, etc.) via `vim.lsp.buf_request()` and return the raw table. [neovim](https://neovim.io/doc/user/lsp/)
- **Response capture**: Return responses as plain Lua tables (JSON-serializable) so an agent consuming output externally can parse them without a Neovim-specific decoder. [zignar](https://zignar.net/2022/10/26/testing-neovim-lsp-plugins/)
- **Skip/pending policy**: Use `pending()` for environment-dependent cases (e.g. missing server binary); never silently skip with `if/else`, per Neovim's own guideline, so pass/fail/pending counts stay consistent across environments. [neovim](https://neovim.io/doc/user/dev_test/)

### Execution Command Pattern
- Local run: `nvim --headless --noplugin -u tests/minimal_init.lua -c "PlenaryBustedDirectory tests/ {minimal_init = 'tests/minimal_init.lua'}"`. [zignar](https://zignar.net/2022/10/26/testing-neovim-lsp-plugins/)
- Alternative (busted+nlua): `busted spec/` with `.busted` config pointing `lua = "nlua"`. [mrcjkb](https://mrcjkb.dev/posts/2023-06-06-luarocks-test.html)

### Out of Scope
- Building a custom JSON-RPC client (use Neovim's built-in `vim.lsp` instead). [neovim](https://neovim.io/doc/user/lsp/)
- UI/screen-based assertions (`Screen.new()`) unless testing editor rendering behavior specifically, not LSP protocol behavior. [neovim](https://neovim.io/doc/user/dev_test/)