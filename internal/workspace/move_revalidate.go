package workspace

import (
	"context"
	"fmt"
	"path/filepath"
)

func revalidateMoveInputs(ctx context.Context, journal *moveJournal) error {
	if len(journal.Files) == 0 {
		return nil
	}
	source := filepath.Join(journal.SourceRoot, journal.SourceRel)
	if err := verifyPathParents(journal.SourceRoot, source); err != nil {
		return err
	}
	verify := verifySnapshotAt
	if journal.WholeFolder {
		verify = verifyExactTree
	}
	if err := verify(source, journal.Files); err != nil {
		return fmt.Errorf("source data changed before ownership publication: %w", err)
	}
	if err := revalidateCommittedSourceSet(ctx, journal); err != nil {
		return err
	}
	if err := revalidateMoveIgnores(ctx, journal); err != nil {
		return err
	}
	return nil
}
