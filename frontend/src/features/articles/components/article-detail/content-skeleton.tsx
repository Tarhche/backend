import {Box, Group, Blockquote, Skeleton, AspectRatio} from "@mantine/core";
import {IconInfoCircle} from "@tabler/icons-react";

export async function ContentSkeleton() {
  return (
    <Box component="article">
      <Skeleton w={"80%"} h={35} />
      <Group wrap="nowrap" c={"dimmed"} my={"sm"} justify="space-between">
        <Group gap={5}>
          <Skeleton w={50} h={20} />
        </Group>
        <Skeleton w={30} h={20} />
      </Group>
      <AspectRatio ratio={16 / 9}>
        <Skeleton w={"100%"} h={"100%"} />
      </AspectRatio>
      <Blockquote
        py={"lg"}
        color="blue"
        radius="md"
        iconSize={30}
        icon={<IconInfoCircle />}
        mt="md"
        mb="xl"
      >
        <Skeleton w={"100%"} h={25} />
      </Blockquote>
      <Skeleton mb={"sm"} w={"70%"} h={30} />
      <Skeleton mb={"sm"} w={"90%"} h={30} />
      <Skeleton mb={"sm"} w={"50%"} h={30} />
      <Skeleton mb={"sm"} w={"75%"} h={30} />
      <Skeleton mb={"sm"} w={"65%"} h={30} />
      <Skeleton mb={"sm"} w={"90%"} h={30} />
      <Skeleton mb={"sm"} w={"100%"} h={30} />
      <Group gap={"xs"} mt={"xl"}>
        <Skeleton w={75} h={30} />
        <Skeleton w={75} h={30} />
        <Skeleton w={75} h={30} />
      </Group>
    </Box>
  );
}
