import {Paper, Stack, Group, Skeleton} from "@mantine/core";

export function UserUpsertFormSkeleton() {
  return (
    <Paper p={"xl"} withBorder>
      <Group justify="center" align="flex-start" gap={"xl"}>
        <Skeleton w={100} h={100} circle />
        <Stack gap={"md"} flex={1}>
          <Skeleton w={"100%"} h={25} />
          <Skeleton w={"100%"} h={25} />
          <Skeleton w={"100%"} h={25} />
          <Skeleton w={"100%"} h={25} />
          <Group justify="flex-end" mt={"lg"}>
            <Skeleton w={100} h={30} />
          </Group>
        </Stack>
      </Group>
    </Paper>
  );
}
