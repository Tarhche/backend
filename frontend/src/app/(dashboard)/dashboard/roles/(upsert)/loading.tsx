import {Paper, Stack, Fieldset, Group, Skeleton} from "@mantine/core";
import {BreadcrumbSkeleton} from "@/components/breadcrumb-skeleton";
import {generateRange} from "@/lib/arrays";

function NewRolePageLoading() {
  return (
    <Stack>
      <BreadcrumbSkeleton />
      <Fieldset>
        <Stack>
          <Skeleton w={"100%"} h={20} />
          <Skeleton w={"100%"} h={20} />
          <Paper p="lg" withBorder>
            <Stack dir="ltr" gap={"xl"}>
              {generateRange(3).map((i) => {
                return (
                  <Stack key={i} dir="ltr">
                    <Skeleton w={100} h={20} />
                    <Group>
                      {generateRange(5).map((i) => {
                        return <Skeleton key={i} radius={"md"} w={90} h={20} />;
                      })}
                    </Group>
                  </Stack>
                );
              })}
            </Stack>
          </Paper>
          <Skeleton w={"100%"} h={100} />
        </Stack>
      </Fieldset>
    </Stack>
  );
}

export default NewRolePageLoading;
