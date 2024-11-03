import Image from "next/image";
import Link from "next/link";
import {notFound} from "next/navigation";
import {
  Title,
  Box,
  Group,
  Text,
  ActionIcon,
  Tooltip,
  Blockquote,
  Badge,
} from "@mantine/core";
import hljs from "highlight.js";
import {
  IconClockHour2,
  IconBookmark,
  IconInfoCircle,
} from "@tabler/icons-react";
import {FILES_PUBLIC_URL} from "@/constants/envs";
import {fetchArticleByUUID} from "@/dal/articles";
import {dateFromNow} from "@/lib/date-and-time";
import classes from "./article-detail.module.css";
import "highlight.js/styles/atom-one-dark.css";

function highlightCode(content: string) {
  const codeRegex = new RegExp(`<code class="(.*?)">(.*?)<\/code>`, "sg");
  // const codeRegex = /<code class="(.*?)">(.*?)<\/code>/g;

  return content.replace(
    codeRegex,
    (match: string, languageClass: string, code: string) => {
      const language = languageClass.replace("language-", "");
      const highlightedCode = hljs.highlight(code, {
        language: language,
        ignoreIllegals: true,
      }).value;
      return `<code class="${languageClass}">${highlightedCode}</code>`;
    },
  );
}

type Props = {
  uuid: string;
};

export async function ArticleDetail({uuid}: Props) {
  const article = await fetchArticleByUUID(uuid);
  const tags = article.tags ?? [];

  if (article === undefined) {
    notFound();
  }

  return (
    <Box>
      <Title>{article.title}</Title>
      <Group
        wrap="nowrap"
        c={"dimmed"}
        my={"sm"}
        // align="center"
        justify="space-between"
      >
        <Group gap={5}>
          <IconClockHour2 spacing={0} size={20} />
          <Text size="sm" c="dimmed" mt={4}>
            {dateFromNow(article.published_at).toString()}
          </Text>
        </Group>
        <Tooltip label="ذخیره کردن" withArrow>
          <ActionIcon variant="transparent" c={"dimmed"} ml={-7}>
            <IconBookmark />
          </ActionIcon>
        </Tooltip>
      </Group>
      <Image
        width={1000}
        height={563}
        src={`${FILES_PUBLIC_URL}/${article.cover}`}
        alt={article.title}
      />
      <Blockquote
        color="blue"
        radius="md"
        iconSize={30}
        icon={<IconInfoCircle />}
        mt="md"
        mb="xl"
      >
        {article.excerpt}
      </Blockquote>
      <Box
        className={classes.content}
        dangerouslySetInnerHTML={{
          __html: highlightCode(article.body),
        }}
      />
      <Group gap={"xs"} mt={"md"}>
        {tags.map((tag: string) => {
          return (
            <Badge
              key={tag}
              variant="filled"
              size="lg"
              color="blue"
              radius="md"
              style={{cursor: "pointer"}}
              component={Link}
              href={`/hashtags/${tag}`}
            >
              {tag}#
            </Badge>
          );
        })}
      </Group>
    </Box>
  );
}
