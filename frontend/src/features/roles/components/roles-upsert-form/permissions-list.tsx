import {Stack, Title, Group, Chip, ChipGroup} from "@mantine/core";
import {fetchAllPermissions} from "@/dal/private/permissions";

type Permission = {name: string; value: string};

type Props = {
  defaultPermissions?: string[];
};

export async function PermissionList({defaultPermissions}: Props) {
  const permissions = (await fetchAllPermissions()).items;

  const transformedPermissions = Object.entries<Permission[]>(
    permissions.reduce((acc: any, curr: any) => {
      const firstSegment = curr.name.split(" ")[0];
      if (acc[firstSegment] !== undefined) {
        acc[firstSegment].push(curr);
      } else {
        acc[firstSegment] = [curr];
      }
      return acc;
    }, {}),
  );

  return (
    <Stack gap="xl" py={"sm"}>
      {transformedPermissions.map(([title, permissions]) => {
        const capitalizedTitle = `${title.slice(0, 1).toLocaleUpperCase()}${title.slice(1)}`;
        return (
          <Stack key={title} dir="ltr" gap={5}>
            <Title order={5}>{capitalizedTitle}</Title>
            <ChipGroup defaultValue={defaultPermissions || []} multiple>
              <Group>
                {permissions.map(({name, value}) => {
                  const capitalizedName = `${name.slice(0, 1).toLocaleUpperCase()}${name.slice(1)}`;
                  return (
                    <Chip
                      type="checkbox"
                      name="permissions"
                      key={name}
                      value={value}
                    >
                      {capitalizedName}
                    </Chip>
                  );
                })}
              </Group>
            </ChipGroup>
          </Stack>
        );
      })}
    </Stack>
  );
}
