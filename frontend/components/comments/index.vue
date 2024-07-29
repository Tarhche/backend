<template>
  <section>
        <h5 class="mb-3">
            <span class="fa-regular fa-comment"></span>
            <span class="mx-1">دیدگاه ها</span>
        </h5>
        <form v-if="params.isLogin" class="my-3 d-flex flex-column" @submit.prevent="sendComment">
            <textarea class="form-control mb-1"  placeholder="دیدگاه خود را اینجا بنویسید" rows="3" required v-model="commentData.body"></textarea>
            <span class="error text-danger" v-if="commentData.error" >متن پیام حداقل باید بیشتر باشد .</span>
            <button class="btn btn-success align-self-start mt-3" type="submit">ثبت دیدگاه</button>
        </form>
        <div v-else class="alert alert-light">
            <i class="fa-regular fa-bell fa-shake fa-xl"></i>
            <span class="mx-1">برای ثبت دیدگاه خود</span>
            <NuxtLink class="mx-1" href="/auth/register">ثبت نام کنید</NuxtLink>
            <span>یا</span>
            <NuxtLink class="mx-1" to="/auth/login">وارد شوید</NuxtLink>
        </div>
    <comments-item v-for="(item , index) in comments" :key="index" :data="item"  v-if="comments && comments.length"/>
  </section>
</template>

<script lang="ts" setup>
const {data} = defineProps(['data'])
const commentData = reactive({
  body:"",
  error:false
})
const comments = (data && data.length) ?  createTree(data) : ""
function createTree(comments:[]) {

  const map = new Map();
  comments.forEach(comment => map.set(comment.uuid, comment));

  const tree:[] = [];
  comments.forEach(comment => {
    const parent = map.get(comment.parent_uuid);
    if (!parent) {
      tree.push(comment);
    }
  });

  // تکمیل کردن زیرمجموعه‌ها
  tree.forEach((node:object) => {
    const fillSubtree = (node) => {
      node.sub = comments.filter(comment => comment.parent_uuid === node.uuid);
      node.sub.forEach(fillSubtree);
    };
    fillSubtree(node);
  });

  return tree;
}
const sendComment = ()=>{
  if (commentData.body.length > 5){
    $fetch(useApiUrlResolver().resolve('comments') , {
      method:'POST',
      body:JSON.stringify({body:commentData.body})
    })
  }
}
const params = reactive({
        isLogin: useAuth().isLogin(),
})
</script>
