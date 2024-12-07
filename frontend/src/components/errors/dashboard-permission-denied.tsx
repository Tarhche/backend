import {Stack, Title} from "@mantine/core";
import {IconLock} from "@tabler/icons-react";

export function PermissionDeniedError() {
  return (
    <Stack align="center" justify="center" h="80%">
      <IconLock size={100} />
      <Title order={3}>شما مجاز به دیدن این صفحه نمی باشید</Title>
    </Stack>
  );
}
