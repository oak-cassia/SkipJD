// normalize-companies is a one-off migration tool that rewrites existing
// job_postings.company values via gamejob.NormalizeCompanyName.
//
// Usage:
//
//	go run ./cmd/normalize-companies            # dry-run (prints diff only)
//	go run ./cmd/normalize-companies --apply    # applies updates
package main

import (
	"flag"
	"fmt"
	"log"

	"gorm.io/gorm"

	"skipjd/internal/cmdutil"
	"skipjd/internal/gamejob"
	"skipjd/internal/model"
)

func main() {
	applyFlag := flag.Bool("apply", false, "Apply changes (default: dry-run)")
	flag.Parse()

	if err := run(*applyFlag); err != nil {
		log.Fatal(err)
	}
}

// run is split from main so log.Fatal cannot skip the deferred cancel.
func run(apply bool) error {
	ctx, cancel := cmdutil.SetupContext(0)
	defer cancel()

	db := cmdutil.MustConnectDB()

	var postings []model.JobPosting
	if err := db.WithContext(ctx).Find(&postings).Error; err != nil {
		return fmt.Errorf("failed to load job_postings: %w", err)
	}

	type change struct {
		id   uint
		from string
		to   string
	}

	changes := make([]change, 0, len(postings))
	for i := range postings {
		normalized := gamejob.NormalizeCompanyName(postings[i].Company)
		if normalized != postings[i].Company {
			changes = append(changes, change{id: postings[i].ID, from: postings[i].Company, to: normalized})
		}
	}

	fmt.Printf("total rows: %d\nchanges: %d\n\n", len(postings), len(changes))
	for _, c := range changes {
		fmt.Printf("  [%d] %q → %q\n", c.id, c.from, c.to)
	}

	if !apply {
		fmt.Println("\n(dry-run) re-run with --apply to persist.")
		return nil
	}

	// Single transaction so a mid-run failure leaves no partial rewrite.
	updated := 0
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, c := range changes {
			result := tx.Model(&model.JobPosting{}).Where("id = ?", c.id).Update("company", c.to)
			if result.Error != nil {
				return fmt.Errorf("update id=%d: %w", c.id, result.Error)
			}
			updated += int(result.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("apply failed (rolled back): %w", err)
	}
	fmt.Printf("\napplied: %d rows updated.\n", updated)
	return nil
}
