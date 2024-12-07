import {Group, Skeleton, PaginationRoot} from "@mantine/core";
import {generateRange} from "@/lib/arrays";

export function PaginationSkeleton() {
  const items = generateRange(5);

  return (
    <PaginationRoot total={5}>
      <Group justify="center" gap={10}>
        {items.map((item) => {
          return <Skeleton key={item} w={30} h={30} />;
        })}
      </Group>
    </PaginationRoot>
  );
}
