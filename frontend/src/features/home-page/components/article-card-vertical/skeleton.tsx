import {Card, Group, Box, Flex, Skeleton} from "@mantine/core";
import {IconClockHour2} from "@tabler/icons-react";
import classes from "./card.module.css";

export function VerticalArticleCardSkeleton() {
  return (
    <Card withBorder radius="md" p={0} className={classes.card}>
      <Flex wrap="nowrap" gap={0} className={classes.group}>
        <Box className={classes.imageWrapper}>
          <Skeleton pos={"absolute"} w={"100%"} height={"100%"} />
        </Box>
        <Box className={classes.body}>
          <Skeleton h={12} w={"100%"} mb={"sm"} />
          <Skeleton h={12} w={"90%"} />
          <Group wrap="nowrap" gap={5} c={"dimmed"} mt={"lg"}>
            <IconClockHour2 spacing={0} size={20} />
            <Skeleton h={8} w={"10%"} />
          </Group>
        </Box>
      </Flex>
    </Card>
  );
}
