import Link from "next/link";
import Image from "next/image";
import {Card, Text, Group, Box, Flex} from "@mantine/core";
import {IconClockHour2} from "@tabler/icons-react";
import {dateFromNow} from "@/lib/date-and-time";
import {FILES_PUBLIC_URL} from "@/constants/envs";
import classes from "./card.module.css";

type Props = {
  article: {
    thumbnail: string;
    title: string;
    subtitle: string;
    publishedDate: string;
    slug: string;
  };
};

export function VerticalArticleCard({article}: Props) {
  return (
    <Card withBorder radius="md" p={0} className={classes.card}>
      <Flex gap={0} className={classes.group}>
        <Box className={classes.imageWrapper}>
          <Image
            fill
            src={`${FILES_PUBLIC_URL}/${article.thumbnail}`}
            alt={article.title}
            style={{
              objectFit: "cover",
            }}
          />
        </Box>
        <Box className={classes.body}>
          <Text
            mt="xs"
            className={classes.title}
            component={Link}
            href={`/articles/${article.slug}`}
          >
            {article.title}
          </Text>
          <Text size="sm" c={"dimmed"} mt={5} mb="md" lineClamp={3}>
            {article.subtitle}
          </Text>
          <Group wrap="nowrap" gap={5} c={"dimmed"}>
            <IconClockHour2 spacing={0} size={20} />
            <Text size="xs" c="dimmed">
              {dateFromNow(article.publishedDate).toString()}
            </Text>
          </Group>
        </Box>
      </Flex>
    </Card>
  );
}
