The uncommitted fix on vHive 88a779314f8e74e9e94d1cd59627f379ab0d0691 changes only root go.mod, go.sum, and go.work.sum. Runtime b4f8c6c2f1ec820ee4e8c2aa8daa3ade98d5f794 and SDK dd9db322432989b96cff7a38a64703d60b6cabf9 pins remain exact. No commit, push, worker, or VM action was performed for this fix.

The committed workspace exposed the legacy genproto root selected through OpenCensus 0.24.0 → gRPC 1.33.2 → genproto 20200526211855-cb27e3aa2013. That monolith contains googleapis/rpc/status, conflicting with the pinned 202607 RPC submodule. The same ambiguity reproduced with fresh GOMODCACHE/GOCACHE and with the warm cache. Earlier local and installed acceptance used GOWORK=off; its standalone graph does not need the root genproto module and still succeeds after the old monolith is downloaded. This establishes a workspace coverage omission, without evidence of cache-file corruption.

A commented canonical replacement maps the old genproto root to upstream v0.0.0-20230526161137-0005af68ea54 (GitHub googleapis/go-genproto commit 0005af68ea5476a8b99ce94ab83ae4aaac864963). This root has no googleapis/rpc directory. The status package is supplied exclusively by the existing pinned RPC submodule. An unused direct requirement was rejected because tidy prunes it; modernizing the old example dependency root did not remove the OpenCensus path and was restored.

Validation completed:

- Fresh-cache original workspace build failed with the exact ambiguous import. Parent independently reran the corrected workspace race build in that isolated cache: exit 0, session 16129, root-clean-workspace-after.log.
- All 663 standalone root package/test dependency entries match before/after: import identities, dependency module versions/replacements, imports, and selected real source-file SHA256s. Synthetic generated test wrapper inputs are unchanged. This fix does not change the accepted standalone consumer compilation closure.
- Root go mod tidy -diff passes with no changes. go work sync retains the root fix unchanged; its unrelated dependency upgrades (including logrus and x/net) in three old child modules were archived and restored, rather than expanding this fix.
- Focused ctriface workspace race tests pass (session 41490); workspace lint reports 0 issues (session 36866); go mod verify reports all modules verified.

The root go.sum delta is +856/-7 lines; all additions are module go.mod metadata checksums retained by normal standalone tidy, not 856 new compiled dependencies. The go.work.sum delta is +1/-60: the post-fix workspace query adds the canonical replacement archive checksum and removes duplicate metadata entries now present in root go.sum. Pre-fix download residue was excluded by rebuilding workspace sums from their committed baseline. Exact sum classification, normalized closures, trial deltas, commands/logs, final file hashes, and patch are under vhive-genproto-fix-evidence/. final-result.json is the concise machine-readable result.

The changed files are stable for parent review and commit. CI rerun remains necessary to confirm the hosted workflow; this local result does not claim a new VM run.
