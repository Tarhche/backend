package migrations

// All is every migration, oldest first. A released migration is never edited,
// renamed or reordered: a database that has applied it will not apply it again.
func All() []Migration {
	return []Migration{
		renameRunnerToWorkload,
		removeTaskStacks,
		replaceTaskPermissions,
		indexWorkloadVMs,
		stacksAsManifests,
		vmsAsManifests,
		snapshotsAsManifests,
		tasksAsManifests,
	}
}
