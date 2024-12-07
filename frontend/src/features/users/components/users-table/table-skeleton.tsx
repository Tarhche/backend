import {Stack, Group, Skeleton} from "@mantine/core";
import {TableSkeleton} from "@/components/skeletons/table";
import {TABLE_HEADERS} from "./table";

export function UsersTableSkeleton() {
  return (
    <Stack>
      <Group justify="flex-end">
        <Skeleton w={100} h={30} />
      </Group>
      <TableSkeleton
        columnsCount={TABLE_HEADERS.length}
        tableProps={{
          verticalSpacing: "sm",
        }}
      />
    </Stack>
  );
}
