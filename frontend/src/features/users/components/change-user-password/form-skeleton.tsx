import {Paper, Stack, Group, Skeleton} from "@mantine/core";

export function FormSkeleton() {
  return (
    <Paper withBorder p="xl">
      <Stack>
        <Skeleton w="100%" h={40} />
        <Skeleton w="100%" h={40} />
        <Group justify="flex-end" mt={"lg"}>
          <Skeleton w="100" h={40} />
        </Group>
      </Stack>
    </Paper>
  );
}
