# Release procedure

1. Update source version, CHANGELOG, release notes under `docs/releases/`, and
   supported documentation. Regenerate the command schema.
2. Run serial lint and the complete release test/coverage gate. Verify examples,
   local docs links, archive contents, and shell syntax. Do not lower gates.
3. Push a JJ bookmark and open a PR against main. Wait for exact-head CI, fix
   failures, then merge through repository controls. Verify main's resulting SHA.
4. Tag that verified main commit as `vVERSION` and push the tag. The Release
   workflow validates source and builds supported Linux assets, includes docs,
   examples and Bash navigation, publishes release notes, and adds SHA256SUMS.
5. Watch every release job to completion. Verify tag/main SHA and release assets,
   download/checksum/extract an archive, and exercise its installed binary.
   Publish completion only after these checks succeed.

The 1.0.0 release supports Linux amd64/arm64/arm/386. Platform builds compile on
hosted runners; native runtime smoke verification is performed on Linux amd64.
Cross-built binaries are not claimed as natively exercised.
