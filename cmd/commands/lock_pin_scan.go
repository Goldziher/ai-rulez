package commands

// lockAcceptFindings is `lock --accept-findings`: pin a source although the
// security scan of its new tree has error findings (see lockrun.Write).
var lockAcceptFindings bool
