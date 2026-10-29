package integrations

// oNoFollow: Windows has no O_NOFOLLOW. The callers check for links with
// Lstat before opening, and os.Root keeps the open inside the root.
const oNoFollow = 0
