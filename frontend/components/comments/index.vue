<template>
  <section>
    <h5 class="mb-3">
      <span class="fa-regular fa-comment"></span>
      <span class="mx-1">دیدگاه ها</span>
    </h5>
    <comments-write-new :disabled="disabled"
                        @sendComment="sendComment" v-if="params.isLogin"/>
    <div v-else class="alert alert-light">
      <i class="fa-regular fa-bell fa-shake fa-xl"></i>
      <span class="mx-1">برای ثبت دیدگاه خود</span>
      <NuxtLink class="mx-1" href="/auth/register">ثبت نام کنید</NuxtLink>
      <span>یا</span>
      <NuxtLink class="mx-1" to="/auth/login">وارد شوید</NuxtLink>
    </div>
    <comments-item v-for="(item , index) in comments" :key="index" :data="item" v-if="comments && comments.length"/>
  </section>
</template>

<script lang="ts" setup>
const route = useRoute()
const {data} = defineProps(['data'])
useState('clearDataAfterCloseComment', ()=>false)
const comments = (data && data.length) ? createTree(data) : ""
const disabled = ref()

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
  // تکمیل کردن زیرمجموعه‌ها
  tree.forEach((node: object) => {
    const fillSubtree = (node: object) => {
      node.sub = comments.filter(comment => comment.parent_uuid === node.uuid);
      node.sub.forEach(fillSubtree);
    };
    fillSubtree(node);
  });
  return tree;
}

const sendComment = async (text: string) => {
  const body = {
    object_uuid: route.params.uuid,
    body: text,
    object_type: 'article',
    parent_uuid: ""
  }
  try {
    disabled.value = true /* برای غیر فعال شدن دکمه ارسال کامنت بعد فشرده شدن تا زمان برشگت درخواست  */
    useState('clearDataAfterCloseComment').value = false /* برای خالی کردن مقدار کامنت در صورت موفقیت بودن درخواست */
    const data = await useUser().$fetch(useApiUrlResolver().resolve('api/comments'), {
      method: 'POST',
      headers: {authorization: `Bearer ${useAuth().accessToken()}`},
      body: JSON.stringify(body),
    })
    useState('clearDataAfterCloseComment').value = true
  } catch (e) {
    console.log(e)
  } finally {
    disabled.value = false /*جواب درخواست ما هرچی که باشه درانتها هر چی که باشه دکمه از حالت disable در میاد*/
  }
}
const params = reactive({
  isLogin: useAuth().isLogin(),
})
</script>
