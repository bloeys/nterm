#version 410

layout(location=0) in vec3 vertPosIn;
layout(location=1) in vec2[4] vertUV0STIn; //[(u,v), (u+sizeU,v), (u,v+sizeV), (u+sizeU,v+sizeV)]
layout(location=5) in vec4 vertColorIn;
layout(location=6) in vec3 modelPos;
layout(location=7) in vec3 modelScale;

out vec2 vertUV0;
out vec4 vertColor;
out vec3 fragPos;

//MVP = Model View Projection
uniform mat4 projViewMat;

void main()
{
    mat4 modelMat = mat4(
        modelScale.x, 0.0, 0.0, 0.0, 
        0.0, modelScale.y, 0.0, 0.0, 
        0.0, 0.0, modelScale.z, 0.0, 
        modelPos.x, modelPos.y, modelPos.z, 1.0
    );

    vertUV0 = vertUV0STIn[gl_VertexID];
    vertColor = vertColorIn;
    fragPos = vec3(modelMat * vec4(vertPosIn, 1.0));

    gl_Position = projViewMat * modelMat * vec4(vertPosIn, 1.0);
}