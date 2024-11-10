import {Paper, Skeleton, Stack, Group} from "@mantine/core";

function EditCommentFormLoading() {
  return (
    <Paper withBorder p={"lg"}>
      <Stack>
        <Skeleton w="100%" h={150} />
        <Skeleton w="100%" h={25} />
        <Group justify="flex-end" mt={"md"}>
          <Skeleton w={90} h={30}></Skeleton>
        </Group>
      </Stack>
    </Paper>
  );
}

export default EditCommentFormLoading;
