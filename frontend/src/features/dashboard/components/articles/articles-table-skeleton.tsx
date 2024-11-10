import Link from "next/link";
import {
  Table,
  TableTr,
  TableTd,
  TableTh,
  TableThead,
  TableTbody,
  ActionIconGroup,
  Group,
  Button,
  Skeleton,
} from "@mantine/core";
import {IconFilePlus} from "@tabler/icons-react";
import {APP_PATHS} from "@/lib/app-paths";

export function ArticlesTableSkeleton() {
  const articles = new Array(5).fill(1).map((_, i) => i);
  return (
    <>
      <Group justify="flex-end">
        <Button
          variant="light"
          component={Link}
          leftSection={<IconFilePlus />}
          href={APP_PATHS.dashboard.articles.new}
        >
          مقاله جدید
        </Button>
      </Group>
      <Table verticalSpacing={"sm"} mt={"sm"}>
        <TableThead>
          <TableTr>
            <TableTh w={"auto"}>#</TableTh>
            <TableTh w={"50%"}>عنوان</TableTh>
            <TableTh w={"25%"}>تاریخ انتشار</TableTh>
            <TableTh w={"25%"}>عملیات</TableTh>
          </TableTr>
        </TableThead>
        <TableTbody>
          {articles.map((article, index) => {
            return (
              <TableTr key={article}>
                <TableTd>{index + 1}</TableTd>
                <TableTd>
                  <Skeleton w={"100%"} h={25} />
                </TableTd>
                <TableTd>
                  <Skeleton w={"100%"} h={25} />
                </TableTd>
                <TableTd>
                  <ActionIconGroup>
                    <Skeleton w={"100%"} h={25} />
                  </ActionIconGroup>
                </TableTd>
              </TableTr>
            );
          })}
        </TableTbody>
      </Table>
    </>
  );
}
