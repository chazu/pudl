package cmd

import "github.com/chazu/pudl/internal/ingestprep"

var importRecordBytes, importDecodedBytes, importStagingBytes int64
var observeRecordBytes, observeDecodedBytes, observeStagingBytes int64

func importIngestLimits() ingestprep.Limits {
	return ingestprep.Limits{RecordBytes: importRecordBytes, DecodedBytes: importDecodedBytes, StagingBytes: importStagingBytes}
}

func observeIngestLimits() ingestprep.Limits {
	return ingestprep.Limits{RecordBytes: observeRecordBytes, DecodedBytes: observeDecodedBytes, StagingBytes: observeStagingBytes}
}

func init() {
	importCmd.Flags().Int64Var(&importRecordBytes, "max-record-bytes", 64<<20, "Maximum decoded JSON record size")
	importCmd.Flags().Int64Var(&importDecodedBytes, "max-decoded-bytes", 1<<30, "Maximum decoded source bytes per file")
	importCmd.Flags().Int64Var(&importStagingBytes, "max-staging-bytes", 2<<30, "Maximum prepared source and record bytes per file")
	ingestObserveCmd.Flags().Int64Var(&observeRecordBytes, "max-record-bytes", 64<<20, "Maximum decoded observation target envelope size")
	ingestObserveCmd.Flags().Int64Var(&observeDecodedBytes, "max-decoded-bytes", 1<<30, "Maximum observation input bytes")
	ingestObserveCmd.Flags().Int64Var(&observeStagingBytes, "max-staging-bytes", 2<<30, "Maximum prepared observation bytes")
}
