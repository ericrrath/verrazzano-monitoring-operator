// Copyright (C) 2020, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
package signals

import (
	"os"
	"syscall"
)

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
