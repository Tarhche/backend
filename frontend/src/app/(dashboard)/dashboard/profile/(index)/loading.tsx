import {Stack, Group, Paper, Skeleton} from "@mantine/core";
import {BreadcrumbSkeleton} from "@/components/breadcrumb-skeleton";

function ProfilePageLoading() {
  return (
    <Stack>
      <BreadcrumbSkeleton />
      <Paper p="lg" withBorder>
        <Group justify="center" align="flex-start">
          <Skeleton w={138} h={138} circle />
          <Stack flex={"1 1 300px"}>
            <Skeleton w={"100%"} h={36} />
            <Skeleton w={"100%"} h={36} />
            <Skeleton w={"100%"} h={36} />
            <Skeleton w={"100%"} h={58} />
            <Group justify="flex-end">
              <Skeleton w={125} h={36} />
            </Group>
          </Stack>
        </Group>
      </Paper>
    </Stack>
  );
}

export default ProfilePageLoading;
