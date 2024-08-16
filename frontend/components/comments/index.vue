<template>
  <section>
    <h5 class="mb-3">
      <span class="fa-regular fa-comment"></span>
      <span class="mx-1">دیدگاه ها</span>
    </h5>

    <template v-if="useAuth().isLogin()">
      <comments-create :objectType="props.objectType" :objectUUID="props.objectUUID"/>
    </template>

    <div v-else class="alert alert-light">
      <i class="fa-regular fa-bell fa-shake fa-xl"></i>
      <span class="mx-1">برای ثبت دیدگاه خود</span>
      <NuxtLink class="mx-1" href="/auth/register">ثبت نام کنید</NuxtLink>
      <span>یا</span>
      <NuxtLink class="mx-1" to="/auth/login">وارد شوید</NuxtLink>
    </div>

    <template v-if="comments && comments.length">
      <comments-item v-for="(item , index) in comments" :key="index" :objectType="props.objectType"
                     :objectUUID="props.objectUUID" :data="item"/>
    </template>
  </section>
</template>

<script setup lang="ts">
const props = defineProps({
  objectType: {
    type: String,
    required: true
  },
  objectUUID: {
    type: String,
    required: true
  },
})

const data = await $fetch(useApiUrlResolver().resolve('api/comments'), {
  query: {
    object_type: props.objectType,
    object_uuid: props.objectUUID,
    page: 1
  }
})

const comments = (data.items && data.items.length) ? createTree(data.items) : []

function createTree(comments: []): [] {
  const map = new Map();
  comments.forEach(comment => map.set(comment.uuid, comment));
  const tree: [] = [];
  comments.forEach(comment => {
    const parent = map.get(comment.parent_uuid);
    if (!parent) {
      tree.push(comment);
    }
  });

  tree.forEach((node: object) => {
    const fillSubtree = (node: object) => {
      node.sub = comments.filter(comment => comment.parent_uuid === node.uuid);
      node.sub.forEach(fillSubtree);
    };
    fillSubtree(node);
  });
  return tree;
}
</script>
