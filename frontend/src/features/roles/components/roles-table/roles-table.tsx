import Link from "next/link";
import {
  Table,
  TableTr,
  TableTd,
  TableTh,
  TableThead,
  TableTbody,
  ActionIcon,
  ActionIconGroup,
  Tooltip,
  Group,
  Button,
  rem,
} from "@mantine/core";
import {Pagination} from "@/components/pagination";
import {PermissionGuard} from "@/components/permission-guard";
import {RoleDeleteButton} from "./role-delete-button";
import {IconPencil, IconPlus} from "@tabler/icons-react";
import {fetchRoles} from "@/dal/private/roles";
import {APP_PATHS} from "@/lib/app-paths";

export const TABLE_HEADERS = ["#", "عنوان", "توضیحات", "عملیات"];

type Props = {
  page: number | string;
};

export async function RolesTable({page}: Props) {
  await new Promise((res) => setTimeout(res, 3000));
  const {items: roles, pagination} = await fetchRoles({
    params: {
      page,
    },
  });

  return (
    <>
      <Group justify="flex-end">
        <Button
          variant="light"
          component={Link}
          href={APP_PATHS.dashboard.roles.new}
          leftSection={<IconPlus />}
        >
          نقش جدید
        </Button>
      </Group>
      <Table verticalSpacing={"sm"} striped withRowBorders>
        <TableThead>
          <TableTr>
            {TABLE_HEADERS.map((h) => {
              return <TableTh key={h}>{h}</TableTh>;
            })}
          </TableTr>
        </TableThead>
        <TableTbody>
          {roles.length === 0 && (
            <TableTr>
              <TableTd colSpan={TABLE_HEADERS.length} ta={"center"}>
                نقشی هنوز وجود ندارد
              </TableTd>
            </TableTr>
          )}
          {roles.map((role: any, index: number) => {
            return (
              <TableTr key={role.uuid}>
                <TableTd>{index + 1}</TableTd>
                <TableTd>{role.name}</TableTd>
                <TableTd>{role.description}</TableTd>
                <TableTd>
                  <ActionIconGroup>
                    <PermissionGuard
                      allowedPermissions={["roles.update", "roles.show"]}
                      operator="AND"
                    >
                      <Tooltip label={"ویرایش کردن نقش"} withArrow>
                        <ActionIcon
                          variant="light"
                          size="lg"
                          color="blue"
                          aria-label="ویرایش کردن نقش"
                          component={Link}
                          href={`${APP_PATHS.dashboard.roles.edit(role.uuid)}`}
                        >
                          <IconPencil style={{width: rem(20)}} stroke={1.5} />
                        </ActionIcon>
                      </Tooltip>
                    </PermissionGuard>
                    <PermissionGuard allowedPermissions={["roles.delete"]}>
                      <RoleDeleteButton
                        roleId={role.uuid}
                        roleName={role.name}
                      />
                    </PermissionGuard>
                  </ActionIconGroup>
                </TableTd>
              </TableTr>
            );
          })}
        </TableTbody>
      </Table>
      {roles.length >= 1 && (
        <Group mt="md" mb={"lg"} justify="flex-end">
          <Pagination
            total={pagination.total_pages}
            current={pagination.current_page}
          />
        </Group>
      )}
    </>
  );
}
