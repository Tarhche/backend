import {Group} from "@mantine/core";
import {TableSkeleton, PaginationSkeleton} from "@/components/skeletons";

export function ArticlesTableSkeleton() {
  return (
    <>
      <TableSkeleton
        tableProps={{
          verticalSpacing: "sm",
          mt: "sm",
        }}
      />
      <Group justify="flex-end" mt="xs">
        <PaginationSkeleton />
      </Group>
    </>
  );
}
