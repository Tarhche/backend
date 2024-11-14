import {Paper} from "@mantine/core";
import {FilesExplorer} from "@/components/files-explorer";

export async function FilesList() {
  return (
    <Paper px="sm" py="sm" withBorder>
      <FilesExplorer />
    </Paper>
  );
}
