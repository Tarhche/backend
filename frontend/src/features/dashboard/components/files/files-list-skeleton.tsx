import {Paper, Skeleton, Stack, Group, Title} from "@mantine/core";

export function FilesListSkeleton() {
  const FILES = new Array(6).fill(1).map((_, i) => i);

  return (
    <Paper withBorder p={"md"}>
      <Stack gap={"md"}>
        <Group justify="space-between">
          <Title order={3}>تصاویر</Title>
          <Skeleton width={40} height={40} />
        </Group>
        <Group>
          {FILES.map((file) => {
            return <Skeleton key={file} width={100} height={100} />;
          })}
        </Group>
        <Group justify="flex-end" gap="xs">
          <Skeleton width={30} height={30} />
          <Skeleton width={30} height={30} />
          <Skeleton width={30} height={30} />
        </Group>
      </Stack>
    </Paper>
  );
}
