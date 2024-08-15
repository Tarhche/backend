<template>
  <section :class="{child: props.data.parent_uuid}" v-if="data">
    <section class="card mb-3">
      <div class="card-body d-flex flex-start">
        <img v-if="props.data.author.avatar" class="rounded-circle shadow-1-strong ms-3" :src="useFilesUrlResolver().resolve(props.data.author.avatar)" alt="avatar" width="65" height="65">
        <div class="flex-grow-1 flex-shrink-1 ">
          <div class="d-flex justify-content-between align-items-center">
            <p class="info mb-1">
              <span v-if="props.data.author.name">{{ props.data.author.name }}</span>
              <span class="text-muted small me-1" v-if="props.data.created_at && props.data.created_at.length">
              <span class="fa-regular fa-clock mx-1"></span>
                <time datetime="">{{ useTime().toAgo(props.data.created_at) }}</time>
              </span>
            </p>
            <div v-if="useAuth().isLogin()" class="text-nowrap">
              <button class="btn text-danger btn-sm" v-if="false">
                <i class="fas fa-trash fa-xs"></i>
              </button>
              <span v-if="false">|</span>
              <button class="btn btn-sm" @click.prevent="toggleShowCommentCreation">
                <i class="fas fa-reply fa-xs"></i>
              </button>
            </div>
          </div>
          <p v-if="props.data.body && props.data.body.length" class="text small mb-0 pt-2 border-top ">{{ props.data.body }}</p>
        </div>
      </div>
    </section>
    <section class="write-comment px-1" ref="createCommentContainer">
      <comments-create @commentCreated="toggleShowCommentCreation" :objectType="props.objectType" :objectUUID="props.objectUUID" :parentUUID="props.data.uuid" :isReplying="true" />
    </section>
    <template v-if="props.data.sub && props.data.sub.length">
      <comments-item v-for="(item , index) in props.data.sub" :key="index" :objectType="props.objectType" :objectUUID="props.objectUUID" :data="item"/>
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
  data: {
    type: Object,
  },
})

const createCommentContainer = ref()

function toggleShowCommentCreation() {
  const element = createCommentContainer.value

  if (element.style.maxHeight) {
    element.style.maxHeight = null
    return
  }

  element.style.maxHeight = element.scrollHeight + 'px'
  element.querySelector("textarea").focus()
}
</script>

<style scoped lang="scss">
card {
  transition: 0.5s;
}

.child {
  margin-right: 3%;
}

p {
  &.text {
    line-height: 30px;
    font-size: 14px;
    color: #6c757d;
  }

  &.info {
    line-height: 20px;
    font-size: 14px;
  }
}

.write-comment {
  max-height: 0;
  overflow: hidden;
  transition: 0.5s !important;
}

@keyframes reply {
  0% {
    transform: translateY(-10%);
    opacity: 0;
  }
  100% {
    transform: translateY(0);
    opacity: 1;
  }
}
</style>
